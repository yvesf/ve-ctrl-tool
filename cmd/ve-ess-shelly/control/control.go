package control

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/yvesf/ve-ctrl-tool/pkg/timemock"
)

var metricControlInput = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
	Name: "ess_pid_input",
	Unit: "watt",
	Help: "The current input for the PID controller",
})

// Run starts the control loop.
// The control loop is blocking and can be stopped by cancelling ctx.
func Run(ctx context.Context, settings Settings, ess ESSControl, meter EnergyMeter) error {
	var (
		pidLastUpdateAt       time.Time
		lastStatsUpdateAt     time.Time
		lastSetpointWrittenAt time.Time
		lastSetpointValue     float64
	)

	pidC := NewPIDWithMetrics(0.15, 0.1, 0.15)
	pidC.SetOutputLimits(-1*float64(settings.MaxWattCharge), float64(settings.MaxWattInverter))

controlLoop:
	for {
		select {
		case <-ctx.Done():
			break controlLoop
		case <-timemock.After(time.Millisecond * 25):
		}

		m, lastMeasurement := meter.LastMeasurement()
		if lastMeasurement.IsZero() || timemock.Now().Sub(lastMeasurement) > time.Second*10 {
			slog.Info("no energy meter information", slog.Time("lastMeasurement", lastMeasurement))
			err := ess.SetZero(ctx)
			if err != nil {
				return err
			}
			continue
		}

		controllerInputM := m.ConsumptionNegative() + float64(settings.PowerOffset)
		metricControlInput.With().Set(controllerInputM)

		if pidLastUpdateAt.IsZero() {
			pidLastUpdateAt = timemock.Now()
		}
		// Take consumption negative to regulate to 0.
		controllerOut := pidC.UpdateDuration(controllerInputM, timemock.Now().Sub(pidLastUpdateAt))
		pidLastUpdateAt = timemock.Now()

		// round PID output to reduce the need for updating the setpoint for marginal changes.
		controllerOut = math.Round(controllerOut/float64(settings.SetpointRounding)) * float64(settings.SetpointRounding)

		// output zero around values +/- 10 around the control point.
		if controllerOut > -1*float64(settings.ZeroPointWindow) && controllerOut < float64(settings.ZeroPointWindow) {
			controllerOut = 0
		}

		// only update the ESS if
		// - value is different from last update.
		// - the value haven't been updated yet.
		// - 15 seconds passed. We have to write about every 30s to not let the ESS shutdown for safety reasons.
		if controllerOut != lastSetpointValue ||
			lastSetpointWrittenAt.IsZero() ||
			timemock.Now().Sub(lastSetpointWrittenAt) > time.Second*20 {
			err := ess.SetpointSet(ctx, int16(controllerOut))
			if err != nil {
				return fmt.Errorf("failed to write ESS setpoint: %w", err)
			}

			lastSetpointValue = controllerOut
			lastSetpointWrittenAt = timemock.Now()
		}

		// collect statistics only every 10 seconds.
		if lastStatsUpdateAt.IsZero() || timemock.Now().Sub(lastStatsUpdateAt) > time.Second*10 {
			_, err := ess.Stats(ctx)
			if err != nil {
				return fmt.Errorf("failed to read ESS stats: %w", err)
			}
			lastStatsUpdateAt = timemock.Now()
		}
	}

	slog.Info("shutdown: reset ESS setpoint to 0")
	ctxSetpoint, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	err := ess.SetpointSet(ctxSetpoint, 0)
	if err != nil {
		return fmt.Errorf("failed to reset ESS setpoint to zero: %w", err)
	}

	return ctx.Err()
}
