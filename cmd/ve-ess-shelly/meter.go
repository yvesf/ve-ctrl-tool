package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/yvesf/ve-ctrl-tool/cmd/ve-ess-shelly/control"
	"github.com/yvesf/ve-ctrl-tool/pkg/backoff"
	"github.com/yvesf/ve-ctrl-tool/pkg/ringbuf"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
	"github.com/yvesf/ve-ctrl-tool/pkg/timemock"
	"golang.org/x/exp/slog"
)

var metricShellyPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
	Name:   "ess_shelly_power",
	Unit:   "watt",
	Help:   "Power readings from shelly device",
	Labels: []string{"meter"},
})

// meter implements the EnergyMeter interface using the Shelly 3 EM.

type meter struct {
	Meter           shelly.Meter
	lock            sync.Mutex
	lastMeasurement control.PowerFlowWatt
	time            time.Time
}

// Run blocks until context is concelled or error occurs.
func (m *meter) Run(ctx context.Context) error {
	const shellyReadInterval = time.Millisecond * 800

	t := timemock.NewTimer(0)
	defer slog.Debug("shelly go-routine done")
	defer t.Stop()

	buf := ringbuf.NewRingbuf(5)
	b := backoff.NewExponentialBackoff(shellyReadInterval, shellyReadInterval*50)
	retry := 0

	for {
		select {
		case <-t.Timer.C:
			value, err := m.Meter.Read()
			if err != nil {
				retry++
				m.lock.Lock()
				m.time = time.Time{} // set invalid
				m.lock.Unlock()
				wait, next := b.Next(retry)
				if !next {
					return fmt.Errorf("meter out of retries: %w", err)
				}
				slog.Error("failed to read from shelly, retry", slog.Duration("wait", wait), slog.Any("err", err))
				t.Reset(wait)
				continue
			}
			retry = 0

			buf.Add(value.TotalPower)
			mean := buf.Mean()
			metricShellyPower.With("totalMean").Set(mean)

			for i, m := range value.EMeters {
				metricShellyPower.With(fmt.Sprintf("meter%d", i)).Set(m.Power)
			}

			m.lock.Lock()
			m.time = timemock.Now()
			m.lastMeasurement = control.ConsumptionPositive(mean)
			m.lock.Unlock()

			t.Reset(shellyReadInterval)
		case <-ctx.Done():
			return nil
		}
	}
}

// LastMeasurement returns the last known power measurement. If time is Zero then value is invalid.
// The "Run" function needs to run within a goroutine to update the value returned here.
func (m *meter) LastMeasurement() (value control.PowerFlowWatt, time time.Time) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.lastMeasurement, m.time
}
