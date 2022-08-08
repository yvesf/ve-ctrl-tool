package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/felixge/pidctrl"
	"github.com/rs/zerolog/log"

	"ve-ctrl-tool/backoff"
	"ve-ctrl-tool/meter"
	"ve-ctrl-tool/victron"
)

func CommandEssShelly(ctx context.Context, mk2 Mk2, args ...string) error {
	const (
		maxWattCharge   = 500.0
		maxWattInverter = 500.0
		testOffset      = -500 // todo: Should be 0 for production mode. -500 means "measure consumption - 500w"
	)
	b := backoff.NewExponentialBackoff(time.Second, time.Second*25)
	wg := sync.WaitGroup{}

	// childCtx is just used to communicate shutdown to go-routines
	childCtx, childCtxCancel := context.WithCancel(context.Background())
	defer childCtxCancel()

	// process Variables
	var (
		setpointSet, meterMeasurement Measurement
		setpointComitted              int16
	)

	// Run loop to send commands to the Multiplus
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			log.Debug().Msg("multiplus go-routine done")
		}()
		ctx := context.Background()

		var errors = 0
	measurementLoop:
		for {
			select {
			case <-childCtx.Done():
				break measurementLoop
			case <-time.After(time.Millisecond * 50):
			}

			var (
				iBat, uBat uint16
				setpoint   int16
			)

			if v, ok := setpointSet.Get(); ok {
				setpoint = int16(v.ConsumptionPositive())
			} else {
				setpoint = 0
			}

			log.Debug().Int16("setpoint", setpoint).Msg("write setpoint to multiplus")

			err := mk2.CommandWriteRAMVarData(ctx, victron.RamIDAssistent129, setpoint)
			if err != nil {
				log.Error().Err(err).Msg("failed to write to RAM 129")
				goto error
			}

			setpointComitted = setpoint
			log.Info().Int16("setpoint-committed", setpointComitted).
				Msg("Multiplus set")

			iBat, uBat, err = mk2.CommandReadRAMVar(ctx, victron.RamIDIBat, victron.RamIDUBat)
			if err != nil {
				log.Error().Err(err).Msg("failed to read IBat")
				goto error
			}
			log.Info().
				Int16("IBat", victron.ParseSigned16(iBat).Int16()).
				Int16("UBat", victron.ParseSigned16(uBat).Int16()).
				Msg("Multiplus Stats")

			errors = 0
			continue

		error:
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
			shellyReadInterval = time.Second * 1
		)
		defer wg.Done()
		var t = time.NewTimer(0)
		defer t.Stop()
		defer func() {
			log.Debug().Msg("shelly go-routine done")
		}()

		buf := NewRingbuf(10)
		shelly := meter.NewShelly3EM(args[0])
		b := backoff.NewExponentialBackoff(shellyReadInterval, shellyReadInterval*50)
		errCount := 0

		for {
			select {
			case <-t.C:
				value, err := shelly.ReadTotalPower()
				if err != nil {
					log.Error().Err(err).Msg("failed to read from shelly")
					errCount++
					meterMeasurement.SetInvalid()
					wait, _ := b.Next(errCount)
					t.Reset(wait)
					continue
				}
				errCount = 0

				buf.Add(value + testOffset)
				meterMeasurement.Set(ConsumptionPositive(buf.Mean()))
				t.Reset(shellyReadInterval)
			case <-childCtx.Done():
				return
			}
		}
	}()

	pidC := pidctrl.NewPIDController(0.01, 0.1, 0.0)
	pidC.SetOutputLimits(-1*maxWattCharge, maxWattInverter)

controlLoop:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			break controlLoop
		case <-childCtx.Done():
			break controlLoop
		default:
		}
		time.Sleep(time.Second * 2)
		m, haveMeasurement := meterMeasurement.Get()
		if !haveMeasurement {
			setpointSet.SetInvalid()
			continue
		}
		controllerOut := pidC.Update(m.ConsumptionNegative()) // Take consumption negative to regulate to 0
		setpointSet.Set(ConsumptionPositive(controllerOut))

		log.Info().
			Str("meter", m.String()).
			Float64("pid-control-out", controllerOut).
			Msg("control loop")
	}

	childCtxCancel() // signal go-routines to exit
	log.Info().Msg("Wait for go-routines")
	wg.Wait()

	log.Info().Msg("reset ESS to 0")
	err := mk2.CommandWriteRAMVarData(context.Background(), victron.RamIDAssistent129, 0)
	if err != nil {
		log.Error().Err(err).Msg("failed to write to RAM 129")
	}

	defer func() {
		log.Info().Msg("ess-shelly finished")
	}()
	return nil
}

type ringbuf struct {
	buf []float64
	p   int
	s   int
}

func NewRingbuf(size int) *ringbuf {
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
