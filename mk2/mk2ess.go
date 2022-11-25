package mk2

import (
	"context"
	"fmt"

	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"

	"github.com/rs/zerolog/log"
)

// ESS is wraps Adapter and adds function to configure the ESS Assistant.
type ESS struct {
	mk2            *Adapter
	assistantRAMID uint16
}

// ESSInit searches for the ESS Assistent in RAM
// if not found returns with error.
func ESSInit(ctx context.Context, mk2 *Adapter) (*ESS, error) {
	// 200 is arbitrary chosen upper bound.
	// should be corrected if information is available.
	for i := 128; i < 200; i++ {
		log.Debug().Int("ramID", i).Msg("probing ramid")
		low, high, _, _, err := mk2.CommandReadRAMVar(ctx, byte(i), 0)
		if err != nil {
			return nil, fmt.Errorf("failed to enumerate ESS assistent ram records: %w", err)
		}
		if high == 0x0 && low == 0x0 {
			log.Debug().Msg("found end of ramIDs in use")
			break
		}

		assistantID := (uint16(high)<<8 | uint16(low)) >> 4
		log.Debug().Int("assistantID", int(assistantID)).Msg("id")
		if assistantID == vebus.AssistantRAMIDESS {
			return &ESS{
				mk2:            mk2,
				assistantRAMID: uint16(i),
			}, nil
		}

		// this is not the ESS record, jump to next block
		i += int(low & 0xf)
	}

	return nil, fmt.Errorf("ESS RAM Record not found")
}

func (m *ESS) SetpointSet(ctx context.Context, value int16) error {
	log.Info().Int("value", int(value)).Int("record", int(m.assistantRAMID)).Msg("write setpoint")
	return m.mk2.CommandWriteRAMVarDataSigned(ctx, m.assistantRAMID+1, value)
}
