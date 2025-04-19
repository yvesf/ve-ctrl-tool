package main

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"

	"github.com/yvesf/ve-ctrl-tool/cmd/ve-ess-shelly/control"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

// inverter implements the ESSControl interface. It supports controlling the setpoint and gathering status information
// on the Victron ESS via mk2.
type inverter struct {
	adapter *mk2.AdapterWithESS
}

func (m inverter) SetpointSet(ctx context.Context, value int16) error {
	return m.adapter.SetpointSet(ctx, value)
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

	return control.EssStats{
		IBat:          float64(iBat) / 10,
		UBat:          float64(uBat) / 100,
		InverterPower: int(inverterPowerRAM),
	}, nil
}
