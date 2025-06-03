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

var (
	metricControlInput = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_input",
		Unit: "watt",
		Help: "The current input for the PID controller",
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
)

// Run starts the control loop.
// The control loop is blocking and can be stopped by cancelling ctx.
func Run(ctx context.Context, settings Settings, ess ESSControl, meter EnergyMeter) error {
	// shared Variables
	var (
		setpointSet           Measurement
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
			setpointSet.SetInvalid()
			metricMultiplusSetpoint.With().Reset(openmetrics.GaugeOptions{})
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
		setpointSet.Set(ConsumptionPositive(controllerOut))

		// only update the ESS if value is different from last update or 15 seconds passed.
		// We have to write about every 30s to not let the ESS shutdown for safety reasons.
		if controllerOut != lastSetpointValue ||
			lastSetpointWrittenAt.IsZero() || timemock.Now().Sub(lastSetpointWrittenAt) > time.Second*20 {
			err := ess.SetpointSet(ctx, int16(controllerOut))
			if err != nil {
				return fmt.Errorf("failed to write ESS setpoint: %w", err)
			}
			metricMultiplusSetpoint.With().Set(float64(controllerOut))

			lastSetpointValue = controllerOut
			lastSetpointWrittenAt = timemock.Now()
		}

		// collect statistics only every 10 seconds.
		if lastStatsUpdateAt.IsZero() || timemock.Now().Sub(lastStatsUpdateAt) > time.Second*10 {
			stats, err := ess.Stats(ctx)
			if err != nil {
				return fmt.Errorf("failed to read ESS stats: %w", err)
			}
			lastStatsUpdateAt = timemock.Now()
			metricMultiplusIBat.With().Set(float64(stats.IBat))
			metricMultiplusUBat.With().Set(float64(stats.UBat))
			metricMultiplusInverterPower.With().Set(float64(stats.InverterPower))
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
