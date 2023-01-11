package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/felixge/pidctrl"
	"github.com/rs/zerolog/log"
	"github.com/yvesf/ve-ctrl-tool/backoff"
	"github.com/yvesf/ve-ctrl-tool/consumer"
)

var now = time.Now

const setpointRounding = 10

type EssStats struct {
	IBat, UBat    float64
	InverterPower int
}

type essControl interface {
	Stats(ctx context.Context) (EssStats, error)
	SetpointSet(ctx context.Context, value int16) error
}

type energyMeter interface {
	Read() (PowerFlowWatt, error)
}

func run(ctx context.Context, ess essControl, shelly energyMeter, consumers consumer.List) {
	wg := sync.WaitGroup{}

	// childCtx is just used to communicate shutdown to go-routines
	childCtx, childCtxCancel := context.WithCancel(context.Background())
	defer childCtxCancel()

	// shared Variables
	var (
		setpointSet, meterMeasurement, inverterPower Measurement
		setpointUpdateAvailable                      = make(chan struct{}, 1)
	)

	// Run loop to send commands to the Multiplus
	wg.Add(1)
	go func() {
		defer func() {
			childCtxCancel()
			log.Debug().Msg("multiplus go-routine done")
			wg.Done()
		}()
		var lastSetpointUpdate, lastRAMIDRead time.Time
		ctx := context.Background()
		b := backoff.NewExponentialBackoff(time.Second, time.Second*50)

		var (
			errors           int
			setpointComitted int16
		)
	measurementLoop:
		for {
			var setpoint int16
			select {
			case <-childCtx.Done():
				break measurementLoop
			case <-time.After(time.Second * 1):
			case <-setpointUpdateAvailable:
			}

			if v, ok := setpointSet.Get(); ok {
				setpoint = int16(v.ConsumptionPositive())
			} else {
				setpoint = 0
			}

			if setpoint != setpointComitted || now().Sub(lastSetpointUpdate) > time.Second*30 {
				log.Debug().Int16("value", setpoint).Msg("write setpoint to multiplus")
				err := ess.SetpointSet(ctx, setpoint)
				if err != nil {
					log.Error().Err(err).Msg("failed to write to ESS RAM")
					goto handleError
				}

				setpointComitted = setpoint
				lastSetpointUpdate = now()
				metricMultiplusSetpoint.With().Set(float64(setpointComitted))
			}

			// update metrics every 5s at max.
			// This is to save time and have it available to update setpoint above when needed.
			if now().Sub(lastRAMIDRead) > time.Second*5 {
				stats, err := ess.Stats(ctx)
				if err != nil {
					goto handleError
				}

				inverterPower.Set(PowerFlowWatt(stats.InverterPower))
				lastRAMIDRead = now()
			}

			errors = 0
			continue

		handleError:
			metricMultiplusInverterPower.With().Reset(openmetrics.GaugeOptions{})
			metricMultiplusUBat.With().Reset(openmetrics.GaugeOptions{})
			metricMultiplusIBat.With().Reset(openmetrics.GaugeOptions{})
			metricMultiplusSetpoint.With().Reset(openmetrics.GaugeOptions{})

			errors++
			sleepDuration, next := b.Next(errors)
			if !next {
				break
			}
			log.Info().Float64("seconds", sleepDuration.Seconds()).Msg("sleep after error")
			select {
			case <-childCtx.Done():
			case <-time.After(sleepDuration):
			}
		}
	}()

	wg.Add(1)
	go func() {
		const shellyReadInterval = time.Millisecond * 800

		t := time.NewTimer(0)
		defer t.Stop()
		defer func() {
			t.Stop()
			childCtxCancel()
			log.Debug().Msg("shelly go-routine done")
			wg.Done()
		}()

		buf := newRingbuf(5)
		b := backoff.NewExponentialBackoff(shellyReadInterval, shellyReadInterval*50)
		errCount := 0

	measurementLoop:
		for {
			select {
			case <-t.C:
				value, err := shelly.Read()
				if err != nil {
					log.Error().Err(err).Msg("failed to read from shelly")
					errCount++
					meterMeasurement.SetInvalid()
					wait, next := b.Next(errCount)
					if !next {
						break measurementLoop
					}
					t.Reset(wait)
					continue
				}
				errCount = 0

				buf.Add(value.ConsumptionPositive())
				mean := buf.Mean()
				metricShellyPower.With("totalMean").Set(mean)
				meterMeasurement.Set(ConsumptionPositive(mean))

				t.Reset(shellyReadInterval)
			case <-childCtx.Done():
				return
			}
		}
	}()

	// timeLastInverterProduction is when the inverter last time generated >= inverterPeakMinimumProduction watt
	var (
		timeLastInverterProduction time.Time
		previousControllerOut      float64
	)

	pidC := pidctrl.NewPIDController(0.05, 0.15, 0.01)
	pidC.SetOutputLimits(-1*(*flagMaxWattCharge), *flagMaxWattInverter)

controlLoop:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			break controlLoop
		case <-childCtx.Done():
			break controlLoop
		case <-time.After(time.Millisecond * 50):
		}
		m, haveMeasurement := meterMeasurement.Get()
		if !haveMeasurement {
			setpointSet.SetInvalid()
			metricControlSetpoint.With().Reset(openmetrics.GaugeOptions{})
			continue
		}
		invertCurrentPower, haveInverterMeasurement := inverterPower.Get()
		if !haveInverterMeasurement {
			invertCurrentPower = 0
		}

		// add current inverter power as feedback onto measurement to reduce oscillation
		controllerInputM := m.ConsumptionNegative() + *flagOffset
		if setpointSet.valid && haveInverterMeasurement {
			// diff is difference between last requested power and actual invert power output
			diff := 0.3 * (setpointSet.value.ConsumptionPositive() - invertCurrentPower.ConsumptionPositive())
			controllerInputM += diff
			metricControlInputDiff.With().Set(diff)
		} else {
			metricControlInputDiff.With().Reset(openmetrics.GaugeOptions{})
		}

		metricControlInput.With().Set(controllerInputM)
		controllerOut := pidC.Update(controllerInputM) // Take consumption negative to regulate to 0
		// round to 10 watt steps. This is to reduce the need for updating the setpoint for marginal changes.
		controllerOut = math.Round(controllerOut/setpointRounding) * setpointRounding
		// don't do anything around +/- 10 around the control point (set ESS to 0)
		if controllerOut > -1*(*flagZeroPointWindow) && controllerOut < *flagZeroPointWindow {
			controllerOut = 0
		}
		setpointSet.Set(ConsumptionPositive(controllerOut))
		if controllerOut != previousControllerOut {
			select { // notify update inverter communication loop
			case setpointUpdateAvailable <- struct{}{}:
			default:
			}
		}
		previousControllerOut = controllerOut
		metricControlSetpoint.With().Set(controllerOut)

		if v, _ := setpointSet.Get(); v.ConsumptionNegative() >= float64(*flagMaxWattCharge)/2.0 {
			timeLastInverterProduction = now()
			pidC.SetOutputLimits(-1*(*flagMaxWattCharge), *flagMaxWattInverterPeak)
		} else if now().Sub(timeLastInverterProduction) > inverterPeakMaximumTimeWindow {
			pidC.SetOutputLimits(-1*(*flagMaxWattCharge), *flagMaxWattInverter)
		}

		min, max := pidC.OutputLimits()
		metricControlPIDMin.With().Set(min)
		metricControlPIDMax.With().Set(max)

		// iterate over consumers in given order. break once first consumer has changed.
		timeLastConsumerChange := consumers.LastChange()
		if timeLastConsumerChange.IsZero() ||
			now().Sub(timeLastConsumerChange) > time.Duration(*flagConsumerDelay)*time.Second {
			for _, c := range consumers {
				err := c.Offer(int(m.ConsumptionPositive()))
				if err != nil {
					log.Error().Err(err).Stringer("consumer", c).Msg("Update of consumer failed")
				}
			}
		}
	}

	childCtxCancel() // signal go-routines to exit
	log.Info().Msg("shutdown: wait for go-routines to finish")
	wg.Wait()

	log.Info().Msg("shutdown: reset ESS setpoint to 0")
	err := ess.SetpointSet(context.Background(), 0)
	if err != nil {
		log.Error().Err(err).Msg("failed to write to ESS Ram")
	}

	defer func() {
		log.Info().Msg("ess-shelly finished")
	}()
}

type ringbuf struct {
	buf []float64
	p   int
	s   int
}

func newRingbuf(size int) *ringbuf {
	return &ringbuf{
		s: size,
	}
}

func (r *ringbuf) Add(v float64) {
	if len(r.buf) < r.s {
		r.buf = append(r.buf, v)
		return
	}
	r.buf[r.p] = v
	r.p = (r.p + 1) % r.s
}

func (r *ringbuf) Mean() float64 {
	var sum float64
	for _, v := range r.buf {
		sum += v
	}
	return sum / float64(len(r.buf))
}

// PowerFlowWatt type represent power that can flow in two directions: Production and Consumption
// Flow is represented by positive/negative values.
type PowerFlowWatt float64

func ConsumptionPositive(watt float64) PowerFlowWatt {
	return PowerFlowWatt(watt)
}

func (p PowerFlowWatt) String() string {
	if p < 0 {
		return fmt.Sprintf("Production(%.2f)", -1*p)
	}
	return fmt.Sprintf("Consumption(%.2f)", p)
}

func (p PowerFlowWatt) ConsumptionPositive() float64 {
	return float64(p)
}

func (p PowerFlowWatt) ConsumptionNegative() float64 {
	return float64(-p)
}

// Measurement is to share PowerFlowWatt values between go-routines.
// like an "Optional" type it can set to hold currently no value.
type Measurement struct {
	m     sync.RWMutex
	value PowerFlowWatt
	valid bool
}

func (o *Measurement) SetInvalid() {
	o.m.Lock()
	defer o.m.Unlock()
	o.valid = false
	o.value = 0.0
}

func (o *Measurement) Set(v PowerFlowWatt) {
	o.m.Lock()
	defer o.m.Unlock()
	o.value = v
	o.valid = true
}

func (o *Measurement) Get() (value PowerFlowWatt, valid bool) {
	o.m.RLock()
	defer o.m.RUnlock()
	if !o.valid {
		return 0.0, false
	}
	return o.value, true
}
