package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/yvesf/ve-ctrl-tool/pkg/ringbuf"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
)

var metricShellyPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
	Name:   "ess_shelly_power",
	Unit:   "watt",
	Help:   "Power readings from shelly device",
	Labels: []string{"meter"},
})

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

// meterReader implements the EnergyMeter interface using the Shelly 3 EM.
type meterReader struct {
	Meter           shelly.Gen2Meter
	lock            sync.Mutex
	lastMeasurement PowerFlowWatt
	time            time.Time
}

// Run blocks until context is concelled or error occurs.
func (m *meterReader) Run(ctx context.Context) error {
	const (
		shellyReadInterval = time.Millisecond * 800
		backoffStart       = shellyReadInterval
		backoffMax         = 50 * shellyReadInterval
	)

	t := time.NewTimer(0)
	defer slog.Debug("meterReader go-routine done")
	defer t.Stop()

	buf := ringbuf.NewRingbuf(5)
	retry := 0

	for {
		select {
		case <-t.C:
			value, err := m.Meter.Read()
			if err != nil {
				retry++
				m.lock.Lock()
				m.time = time.Time{} // set invalid
				m.lock.Unlock()

				wait := time.Duration((1.0+rand.Float64())* // random 1..2
					float64(backoffStart.Milliseconds())*
					math.Pow(2, float64(retry))) * time.Millisecond
				if wait >= backoffMax {
					return fmt.Errorf("meterReader out of retries: %w", err)
				}
				slog.Error("failed to read from shelly, retry", slog.Duration("wait", wait), slog.Any("err", err))
				t.Reset(wait)
				continue
			}
			retry = 0

			buf.Add(value.TotalPower())
			mean := buf.Mean()
			metricShellyPower.With("totalMean").Set(mean)

			m.lock.Lock()
			m.time = time.Now()
			m.lastMeasurement = ConsumptionPositive(mean)
			m.lock.Unlock()

			t.Reset(shellyReadInterval)
		case <-ctx.Done():
			return nil
		}
	}
}

// LastMeasurement returns the last known power measurement. If time is Zero then value is invalid.
// The "Run" function needs to run within a goroutine to update the value returned here.
func (m *meterReader) LastMeasurement() (value PowerFlowWatt, time time.Time) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.lastMeasurement, m.time
}
