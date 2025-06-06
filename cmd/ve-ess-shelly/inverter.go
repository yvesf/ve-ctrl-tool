package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bsm/openmetrics"
	"github.com/yvesf/ve-ctrl-tool/cmd/ve-ess-shelly/control"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

var (
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

// inverter implements the ESSControl interface. It supports controlling the setpoint and gathering status information
// on the Victron ESS via mk2.
type inverter struct {
	adapter *mk2.AdapterWithESS
}

func (m inverter) SetpointSet(ctx context.Context, value int16) error {
	metricMultiplusSetpoint.With().Set(float64(value))
	return m.adapter.SetpointSet(ctx, value)
}

func (m inverter) SetZero(ctx context.Context) error {
	err := m.adapter.SetpointSet(ctx, 0)
	if err != nil {
		return err
	}
	metricMultiplusSetpoint.With().Reset(openmetrics.GaugeOptions{})
	return nil
}

func (m inverter) Stats(ctx context.Context) (control.EssStats, error) {
	iBat, uBat, err := m.adapter.CommandReadRAMVarSigned16(ctx, vebus.RAMIDIBat, vebus.RAMIDUBat)
	if err != nil {
		return control.EssStats{}, fmt.Errorf("failed to read IBat/UBat: %w", err)
	}

	inverterPowerRAM, _, err := m.adapter.CommandReadRAMVarSigned16(ctx, vebus.RAMIDInverterPower1, 0)
	if err != nil {
		return control.EssStats{}, fmt.Errorf("failed to read InverterPower1: %w", err)
	}

	slog.Debug("multiplus stats", slog.Float64("IBat", float64(iBat)/10),
		slog.Float64("UBat", float64(uBat)/100),
		slog.Float64("InverterPower", float64(inverterPowerRAM)))

	stats := control.EssStats{
		IBat:          float64(iBat) / 10,
		UBat:          float64(uBat) / 100,
		InverterPower: int(inverterPowerRAM),
	}

	metricMultiplusIBat.With().Set(float64(stats.IBat))
	metricMultiplusUBat.With().Set(float64(stats.UBat))
	metricMultiplusInverterPower.With().Set(float64(stats.InverterPower))

	return stats, nil
}
