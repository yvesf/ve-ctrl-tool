package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
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
	metricMultiplusSoC = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_soc",
		Unit: "percent",
		Help: "Ram ChargeState (SoC) in percent",
	})
)

type Controller struct {
	ess   *mk2.AdapterWithESS
	meter *meterReader
	statusIBat,
	statusUBat,
	statusInverterPower,
	statusSoC,
	statusSetpoint *float64
	settingBatteryCapacityKWh float64
}

// Run starts the control loop.
// The control loop is blocking and can be stopped by cancelling ctx.
func (c *Controller) Run(ctx context.Context) error {
	var (
		pidLastUpdateAt       time.Time
		lastStatsUpdateAt     time.Time
		lastSetpointWrittenAt time.Time
		lastSetpointValue     float64
	)

	var err error
	c.settingBatteryCapacityKWh, err = c.ess.ReadBatteryCapacity(ctx)
	if err != nil {
		return fmt.Errorf("reading battery capacity: %w", err)
	}

	pidC := NewPIDWithMetrics(0.15, 0.1, 0.15)

controlLoop:
	for {
		select {
		case <-ctx.Done():
			break controlLoop
		case <-time.After(time.Millisecond * 25):
		}

		pidC.SetOutputLimits(-1*float64(*SettingsMaxWattCharge), float64(*SettingsMaxWattInverter))

		m, lastMeasurement := c.meter.LastMeasurement()
		if lastMeasurement.IsZero() || time.Since(lastMeasurement) > time.Second*10 {
			// Set ESS to 0.
			if lastSetpointValue != 0 ||
				lastSetpointWrittenAt.IsZero() ||
				time.Since(lastSetpointWrittenAt) > time.Second*5 {
				slog.Info("no energy meter information", slog.Time("lastMeasurement", lastMeasurement))

				err := c.SetpointSet(ctx, 0)
				if err != nil {
					return err
				}

				lastSetpointValue = 0
				lastSetpointWrittenAt = time.Now()
			}
			continue
		}

		controllerInputM := m.ConsumptionNegative() + float64(*SettingsPowerOffset)
		metricControlInput.With().Set(controllerInputM)

		if pidLastUpdateAt.IsZero() {
			pidLastUpdateAt = time.Now()
		}
		// Take consumption negative to regulate to 0.
		controllerOut := pidC.UpdateDuration(controllerInputM, time.Since(pidLastUpdateAt))
		pidLastUpdateAt = time.Now()

		// round PID output to reduce the need for updating the setpoint for marginal changes.
		controllerOut = math.Round(controllerOut/float64(*SettingsSetpointRounding)) * float64(*SettingsSetpointRounding)

		// output zero around values +/- 10 around the control point.
		if controllerOut > -1*float64(*SettingsZeroPointWindow) && controllerOut < float64(*SettingsZeroPointWindow) {
			controllerOut = 0
		}

		// only update the ESS if
		// - value is different from last update.
		// - the value haven't been updated yet.
		// - 15 seconds passed. We have to write about every 30s to not let the ESS shutdown for safety reasons.
		if controllerOut != lastSetpointValue ||
			lastSetpointWrittenAt.IsZero() ||
			time.Since(lastSetpointWrittenAt) > time.Second*20 {
			err := c.SetpointSet(ctx, int16(controllerOut))
			if err != nil {
				return fmt.Errorf("failed to write ESS setpoint: %w", err)
			}

			lastSetpointValue = controllerOut
			lastSetpointWrittenAt = time.Now()
		}

		// collect statistics only every 10 seconds.
		if lastStatsUpdateAt.IsZero() || time.Since(lastStatsUpdateAt) > time.Second*10 {
			err := c.updateStatus(ctx)
			if err != nil {
				return fmt.Errorf("failed to read ESS stats: %w", err)
			}
			lastStatsUpdateAt = time.Now()
		}
	}

	slog.Info("shutdown: reset ESS setpoint to 0")
	ctxSetpoint, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	err = c.SetpointSet(ctxSetpoint, 0)
	if err != nil {
		return fmt.Errorf("failed to reset ESS setpoint to zero: %w", err)
	}

	return ctx.Err()
}

func (c *Controller) SetpointSet(ctx context.Context, value int16) error {
	err := c.ess.SetpointSet(ctx, value)
	if err != nil {
		c.statusSetpoint = nil
		return err
	}
	c.statusSetpoint = ptrTo(float64(value))
	c.updateMetrics()
	return nil
}

func (c *Controller) updateStatus(ctx context.Context) error {
	defer c.updateMetrics()

	iBat, uBat, err := c.ess.CommandReadRAMVarSigned16(ctx, vebus.RAMIDIBat, vebus.RAMIDUBat)
	if err != nil {
		c.statusIBat = nil
		c.statusUBat = nil
		c.statusInverterPower = nil
		c.statusSoC = nil
		return fmt.Errorf("failed to read IBat/UBat: %w", err)
	}

	c.statusIBat = ptrTo(float64(iBat) / 10.0)
	c.statusUBat = ptrTo(float64(uBat) / 100.0)

	inverterPower, chargeState, err := c.ess.CommandReadRAMVarSigned16(ctx, vebus.RAMIDInverterPower1, vebus.RAMIDChargeState)
	if err != nil {
		c.statusInverterPower = nil
		c.statusSoC = nil
		return fmt.Errorf("failed to read InverterPower1: %w", err)
	}
	c.statusInverterPower = ptrTo(float64(inverterPower))
	c.statusSoC = ptrTo(float64(chargeState) / 2.0)

	return nil
}

func (c *Controller) updateMetrics() {
	if v := c.statusUBat; v != nil {
		metricMultiplusUBat.With().Set(*v)
	} else {
		metricMultiplusUBat.With().Reset(openmetrics.GaugeOptions{})
	}
	if v := c.statusIBat; v != nil {
		metricMultiplusIBat.With().Set(*v)
	} else {
		metricMultiplusIBat.With().Reset(openmetrics.GaugeOptions{})
	}
	if v := c.statusInverterPower; v != nil {
		metricMultiplusInverterPower.With().Set(*v)
	} else {
		metricMultiplusInverterPower.With().Reset(openmetrics.GaugeOptions{})
	}
	if v := c.statusSoC; v != nil {
		metricMultiplusSoC.With().Set(*v)
	} else {
		metricMultiplusSoC.With().Reset(openmetrics.GaugeOptions{})
	}
	if v := c.statusSetpoint; v != nil {
		metricMultiplusSetpoint.With().Set(*v)
	} else {
		metricMultiplusSetpoint.With().Reset(openmetrics.GaugeOptions{})
	}
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		err := json.NewEncoder(w).Encode(struct {
			IBat               *float64 `json:"iBat"`
			UBat               *float64 `json:"uBat"`
			InverterPower      *float64 `json:"inverterPower"`
			SoC                *float64 `json:"soc"`
			Setpoint           *float64 `json:"setpoint"`
			BatteryCapacityKWh float64  `json:"batteryCapacityKWh"`
		}{
			IBat:               c.statusIBat,
			UBat:               c.statusUBat,
			InverterPower:      c.statusInverterPower,
			SoC:                c.statusSoC,
			Setpoint:           c.statusSetpoint,
			BatteryCapacityKWh: c.settingBatteryCapacityKWh,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}

func ptrTo[T any](v T) *T {
	return &v
}
