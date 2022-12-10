package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/yvesf/ve-ctrl-tool/backoff"
	"github.com/yvesf/ve-ctrl-tool/cmd"
	"github.com/yvesf/ve-ctrl-tool/meter"
	"github.com/yvesf/ve-ctrl-tool/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"

	"github.com/bsm/openmetrics"
	"github.com/bsm/openmetrics/omhttp"
	"github.com/felixge/pidctrl"
	"github.com/rs/zerolog/log"
)

var (
	metricControlSetpoint = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_setpoint",
		Unit: "watt",
		Help: "The current setpoint calculated by the PID controller",
	})
	metricControlInput = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_input",
		Unit: "watt",
		Help: "The current input for the PID controller",
	})
	metricControlInputDiff = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_input_diff",
		Unit: "watt",
		Help: "The current input difference for stabilization of the PID controller",
	})
	metricControlPIDMin = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_output_min",
		Unit: "watt",
		Help: "PID min",
	})
	metricControlPIDMax = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_output_max",
		Unit: "watt",
		Help: "PID max",
	})
	metricMultiplusSetpoint = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_setpoint",
		Unit: "watt",
		Help: "The setpoint written to the multiplus",
	})
	metricMultiplusIBat = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_ibat",
		Unit: "ampere",
		Help: "Current of the multiplus battery, negative=discharge",
	})
	metricMultiplusUBat = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_ubat",
		Unit: "voltage",
		Help: "Voltage of the multiplus battery",
	})
	metricMultiplusInverterPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_inverter_power",
		Unit: "watt",
		Help: "Ram InverterPower1",
	})
	metricShellyPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name:   "ess_shelly_power",
		Unit:   "watt",
		Help:   "Power readings from shelly device",
		Labels: []string{"meter"},
	})
)

const (
	inverterPeakMaximumTimeWindow = time.Minute * 15 // in the last 15min.. for max 15min
)

var (
	flagMetricsHTTP     = flag.String("metricsHTTP", "", "Address of a http server serving metrics under /metrics")
	flagMaxWattCharge   = flag.Float64("maxCharge", 250.0, "Maximum ESS Setpoint for charging (negative setpoint)")
	flagMaxWattInverter = flag.Float64("maxInverter",
		60.0,
		"Maximum ESS Setpoint for inverter (positive setpoint)")
	flagMaxWattInverterPeak = flag.Float64("maxInverterPeak",
		800,
		"Maximum ESS Setpoint for inverter (positive setpoint) for peaks after recent peak charging phase")
	flagOffset          = flag.Float64("offset", -10.0, "Power measurement offset")
	flagZeroPointWindow = flag.Float64("zeroWindow", 20, "Do not operate if measurement is in this +/- window")
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	adapter := cmd.CommonInit(ctx)

	// Metrics HTTP endpoint
	if *flagMetricsHTTP != `` {
		mux := http.NewServeMux()
		mux.Handle("/metrics", omhttp.NewHandler(openmetrics.DefaultRegistry()))

		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", *flagMetricsHTTP)
		if err != nil {
			log.Panic().Err(err).Str("addr", *flagMetricsHTTP).Msg("Listen on http failed")
		}

		srv := &http.Server{Handler: mux}
		go func() {
			err := srv.Serve(ln)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("http server failed")
			}
		}()
	}

	mk2Ess, err := mk2.ESSInit(ctx, adapter)
	if err != nil {
		panic(err)
	}

	wg := sync.WaitGroup{}

	// childCtx is just used to communicate shutdown to go-routines
	childCtx, childCtxCancel := context.WithCancel(context.Background())
	defer childCtxCancel()

	// shared Variables
	var setpointSet, meterMeasurement, inverterPower Measurement

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
			errors           = 0
			setpointComitted int16
		)
	measurementLoop:
		for {
			var (
				iBat, uBat, inverterPowerRAM, setpoint int16
				err                                    error
			)

			select {
			case <-childCtx.Done():
				break measurementLoop
			case <-time.After(time.Millisecond * 50):
			}

			if v, ok := setpointSet.Get(); ok {
				setpoint = int16(v.ConsumptionPositive())
			} else {
				setpoint = 0
			}

			if setpoint != setpointComitted || time.Since(lastSetpointUpdate) > time.Second*30 {
				log.Debug().Int16("setpoint", setpoint).Msg("write setpoint to multiplus")
				err := mk2Ess.SetpointSet(ctx, setpoint)
				if err != nil {
					log.Error().Err(err).Msg("failed to write to ESS RAM")
					goto handleError
				}

				setpointComitted = setpoint
				lastSetpointUpdate = time.Now()
				log.Info().Int16("setpoint-committed", setpointComitted).
					Msg("Multiplus set")
				metricMultiplusSetpoint.With().Set(float64(setpointComitted))
			}

			// update metrics every 5s at max.
			// This is to save time and have it available to update setpoint.
			if time.Since(lastRAMIDRead) > time.Second*5 {
				iBat, uBat, err = adapter.CommandReadRAMVarSigned16(ctx, vebus.RAMIDIBat, vebus.RAMIDUBat)
				if err != nil {
					log.Error().Err(err).Msg("failed to read IBat/UBat")
					goto handleError
				}
				inverterPowerRAM, _, err = adapter.CommandReadRAMVarSigned16(ctx, vebus.RAMIDInverterPower1, 0)
				if err != nil {
					log.Error().Err(err).Msg("failed to read InverterPower1")
					goto handleError
				}
				log.Debug().Float32("IBat", float32(iBat)/10).
					Float32("UBat", float32(uBat)/100).
					Float32("InverterPower", float32(inverterPowerRAM)).
					Msg("Multiplus Stats")
				metricMultiplusIBat.With().Set(float64(iBat) / 10)
				metricMultiplusUBat.With().Set(float64(uBat) / 100)
				metricMultiplusInverterPower.With().Set(float64(inverterPowerRAM))
				inverterPower.Set(PowerFlowWatt(inverterPowerRAM))
				lastRAMIDRead = time.Now()
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
		const (
			shellyReadInterval = time.Millisecond * 800
		)

		t := time.NewTimer(0)
		defer t.Stop()
		defer func() {
			t.Stop()
			childCtxCancel()
			log.Debug().Msg("shelly go-routine done")
			wg.Done()
		}()

		buf := newRingbuf(5)
		shelly := meter.NewShelly3EM(flag.Args()[0])
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

				metricShellyPower.With("total").Set(value.TotalPower)
				for i, m := range value.EMeters {
					metricShellyPower.With(fmt.Sprintf("emeter%d", i)).Set(m.Power)
				}

				buf.Add(value.TotalPower)
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
	var timeLastInverterProduction time.Time

	pidC := pidctrl.NewPIDController(0.05, 0.15, 0.01)
	pidC.SetOutputLimits(-1*(*flagMaxWattCharge), *flagMaxWattInverter)

controlLoop:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			break controlLoop
		case <-childCtx.Done():
			break controlLoop
		case <-time.After(time.Millisecond * 250):
		}
		m, haveMeasurement := meterMeasurement.Get()
		if !haveMeasurement {
			setpointSet.SetInvalid()
			metricControlSetpoint.With().Reset(openmetrics.GaugeOptions{})
			continue
		}
		invertCurrentPower, haveMeasurement := inverterPower.Get()
		if !haveMeasurement {
			invertCurrentPower = 0
		}

		// add current inverter power as feedback onto measurement to reduce oscillation
		controllerInputM := m.ConsumptionNegative() + *flagOffset
		if setpointSet.valid {
			diff := 0.6 * (setpointSet.value.ConsumptionPositive() + invertCurrentPower.ConsumptionPositive())
			controllerInputM += diff
			metricControlInputDiff.With().Set(diff)
		} else {
			metricControlInputDiff.With().Reset(openmetrics.GaugeOptions{})
		}

		metricControlInput.With().Set(controllerInputM)
		controllerOut := pidC.Update(controllerInputM) // Take consumption negative to regulate to 0
		// round to 10 watt steps. This is to reduce the need for updating the setpoint for marginal changes.
		controllerOut = math.Round(controllerOut/10) * 10
		// don't do anything around +/- 10 around the control point (set ESS to 0)
		if controllerOut > -1*(*flagZeroPointWindow) && controllerOut < *flagZeroPointWindow {
			controllerOut = 0
		}
		setpointSet.Set(ConsumptionPositive(controllerOut))
		metricControlSetpoint.With().Set(controllerOut)

		if v, _ := setpointSet.Get(); v.ConsumptionNegative() >= float64(*flagMaxWattCharge)/2.0 {
			timeLastInverterProduction = time.Now()
			pidC.SetOutputLimits(-1*(*flagMaxWattCharge), *flagMaxWattInverterPeak)
		} else if time.Since(timeLastInverterProduction) > inverterPeakMaximumTimeWindow {
			pidC.SetOutputLimits(-1*(*flagMaxWattCharge), *flagMaxWattInverter)
		}

		min, max := pidC.OutputLimits()
		metricControlPIDMin.With().Set(min)
		metricControlPIDMax.With().Set(max)
	}

	childCtxCancel() // signal go-routines to exit
	log.Info().Msg("Wait for go-routines")
	wg.Wait()

	log.Info().Msg("reset ESS setpoint to 0")
	err = mk2Ess.SetpointSet(context.Background(), 0)
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
