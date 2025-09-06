package mk2

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

type Simulator struct {
	Port            Port
	RamVars         [256]*int16
	WSelectedRamVar int
}

func (s *Simulator) Run(ctx context.Context) {
	mk2 := NewReader(s.Port)
	err := mk2.StartReader(ctx, false)
	if err != nil {
		panic(err)
	}

	ch := mk2.NewListenChannel()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case frame := <-ch:
				switch frame[2] {
				case byte(vebus.CommandR):
					slog.InfoContext(ctx, "reset received")
				case byte(vebus.CommandN):
					slog.InfoContext(ctx, "upgrade notice received")
				case byte(vebus.CommandA):
					slog.InfoContext(ctx, "Address select received")
					b := vebus.CommandA.Frame(frame[3], 0x00, 0x00)
					mk2.WriteContext(ctx, b.Marshal())
				case byte(vebus.CommandW):
					wCommand := frame[3]
					switch wCommand {
					case byte(vebus.WCommandReadRAMVar):
						ramIDs := vebus.RAMIDs(frame[4:])
						slog.InfoContext(ctx, "read RAM Var", slog.Any("addresses", fmt.Sprintf("%+v", ramIDs)))
						b := vebus.CommandW.Frame(0x85) // , 0x05<<4, 0x00, 0x00, 0x00
						for _, ramID := range ramIDs {
							switch ramID {
							case vebus.RAMIDUMainsRMS:
								b.Data = append(b.Data, 0x00, 0x00)
							case vebus.RAMIDUBat:
								b.Data = append(b.Data, 0x00, 0x00)
							case vebus.RAMIDIBat:
								b.Data = append(b.Data, 0x00, 0x00)
							case vebus.RAMIDInverterPower1:
								b.Data = append(b.Data, 0x00, 0x00)
							case 128: // ESS Assistant.
								b.Data = append(b.Data, 0x05<<4, 0x00)
							default:
								slog.InfoContext(ctx, "placeholder 0x00,0x00 for ramid", slog.Any("ramId", ramID))
								b.Data = append(b.Data, 0x00, 0x00)
							}
						}
						mk2.WriteContext(ctx, b.Marshal())
					case byte(vebus.WCommandWriteRAMVar):
						address := vebus.ParseSigned16Bytes(frame[4], frame[5])
						s.WSelectedRamVar = int(address)
						slog.InfoContext(ctx, "received WCommandWriteRAMVar", slog.Any("address", address))
					case byte(vebus.WCommandWriteData):
						value := vebus.ParseSigned16Bytes(frame[4], frame[5])
						slog.InfoContext(ctx, "received WriteData", slog.Any("value", value))
						s.RamVars[s.WSelectedRamVar] = &value
						b := vebus.CommandW.Frame(0x87, 0x00, 0x00) // "Just ignore the rest of the frame (00,00 in this case)) "
						mk2.WriteContext(ctx, b.Marshal())
					}
				default:
					slog.ErrorContext(ctx, "unhandled frame received", slog.Any("frame", hex.EncodeToString(frame)))
				}
			case <-time.After(time.Second * 2):
				b := vebus.Command('V').Frame('$', 0xdb, 0x11, 0x00, 0x00)
				mk2.WriteContext(ctx, b.Marshal())
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	mk2.Shutdown()
}
