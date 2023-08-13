package mk2

import (
	"context"
	"fmt"

	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
	"golang.org/x/exp/slog"
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
		slog.Debug("probing ramid", slog.Int("ramID", i))
		low, high, _, _, err := mk2.CommandReadRAMVar(ctx, byte(i), 0)
		if err != nil {
			return nil, fmt.Errorf("failed to enumerate ESS assistent ram records: %w", err)
		}
		if high == 0x0 && low == 0x0 {
			slog.Debug("found end of ramIDs in use")
			break
		}

		assistantID := (uint16(high)<<8 | uint16(low)) >> 4
		slog.Debug("id", slog.Int("assistantID", int(assistantID)))
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
