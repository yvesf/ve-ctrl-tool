package mk2

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

// AdapterWithESS is wraps Adapter and adds function to configure the AdapterWithESS Assistant.
type AdapterWithESS struct {
	*Adapter
	assistantRAMID uint16
}

// ESSInit searches for the ESS Assistent in RAM
// if not found returns with error.
func ESSInit(ctx context.Context, mk2 *Adapter) (*AdapterWithESS, error) {
	// 200 is arbitrary chosen upper bound.
	// should be corrected if information is available.
	for i := 128; i < 200; i++ {
		low, high, _, _, err := mk2.CommandReadRAMVar(ctx, vebus.RAMID(i), 0)
		if err != nil {
			return nil, fmt.Errorf("failed to enumerate ESS assistent ram records: %w", err)
		}
		if high == 0x0 && low == 0x0 {
			slog.Debug("found end of ramIDs in use")
			break
		}
		// The ID_Size word contains 0xZZZY, where:
		// ZZZ = the Assistant ID. This is where you can recognize the function of the assistant (ESS ID = 5).
		// Y = the number of ramIDs that will follow after the ID_Size.
		assistantID := (uint16(high)<<8 | uint16(low)) >> 4
		slog.Debug("id", slog.Int("assistantID", int(assistantID)), slog.Int("ramID", i))
		if assistantID == vebus.AssistantRAMIDESS {
			return &AdapterWithESS{
				Adapter:        mk2,
				assistantRAMID: uint16(i),
			}, nil
		}

		// this is not the ESS record, jump to next block
		i += int(low & 0xf)
	}

	return nil, fmt.Errorf("ESS RAM Record not found")
}

func (m *AdapterWithESS) SetpointSet(ctx context.Context, value int16) error {
	slog.Info("write setpoint", slog.Int("value", int(value)), slog.Int("record", int(m.assistantRAMID)))
	return m.CommandWriteRAMVarDataSigned(ctx, m.assistantRAMID+1, value)
}

func (m *AdapterWithESS) ReadBatteryCapacity(ctx context.Context) (float64, error) {
	capacityLow, capacityHigh, err := m.CommandReadSetting(ctx, vebus.SettingIDBatteryCapacity)
	if err != nil {
		return 0.0, fmt.Errorf("reading battery capacity: %w", err)
	}
	capacityAh := float64(uint16(capacityLow) | (uint16(capacityHigh) << 8))

	floatVoltageLow, floatVoltageHigh, err := m.CommandReadSetting(ctx, vebus.SettingIDUBatFloat)
	if err != nil {
		return 0.0, fmt.Errorf("reading float voltage: %w", err)
	}
	floatVoltage := float64(vebus.ParseSigned16Bytes(floatVoltageLow, floatVoltageHigh)) / 100.0

	capacityKWh := (capacityAh * floatVoltage) / 1000
	// approximation: reduce capacity to match realistic capacity.
	capacityKWh = capacityKWh * 0.9

	return capacityKWh, nil
}
