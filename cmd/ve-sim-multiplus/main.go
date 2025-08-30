package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/containerd/console"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

var flagDebug = flag.Bool("debug", false, "Set log level to debug")

func main() {
	logLevel := slog.LevelInfo
	if *flagDebug {
		logLevel = slog.LevelDebug
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(h))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Open a new PTY as a virtual serial port.
	c, slave, err := console.NewPty()
	if err != nil {
		panic(err)
	}
	defer c.Close()
	err = c.SetRaw()
	if err != nil {
		panic(err)
	}
	slog.Info(fmt.Sprint("Port: ", slave))

	go func() {
		<-ctx.Done()
		c.Close()
	}()

	port := &mk2.FilePort{
		ReadWriter: c,
	}
	mk2 := mk2.NewReader(port)
	err = mk2.StartReader(false)
	if err != nil {
		panic(err)
	}

	println("listening")
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
					slog.Info("Reset received")
				case byte(vebus.CommandN):
					slog.Info("upgrade notice received")
				case byte(vebus.CommandA):
					slog.Info("Address select received")
					b := vebus.CommandA.Frame(frame[3], 0x00, 0x00)
					mk2.Write(b.Marshal())
				case byte(vebus.CommandW):
					wCommand := frame[3]
					switch wCommand {
					case byte(vebus.WCommandReadRAMVar):
						ramIDs := vebus.RAMIDs(frame[4:])
						slog.Info("Read RAM Var", slog.Any("addresses", fmt.Sprintf("%+v", ramIDs)))
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
								slog.Info("placeholder 0x00,0x00 for ramid", slog.Any("ramId", ramID))
								b.Data = append(b.Data, 0x00, 0x00)
							}
						}
						mk2.Write(b.Marshal())
					case byte(vebus.WCommandWriteRAMVar):
						address := vebus.ParseSigned16Bytes(frame[4], frame[5])
						slog.Info("WCommandWriteRAMVar", slog.Any("address", address))
					case byte(vebus.WCommandWriteData):
						value := vebus.ParseSigned16Bytes(frame[4], frame[5])
						slog.Info("WriteData", slog.Any("value", value))
						b := vebus.CommandW.Frame(0x87, 0x00, 0x00) // "Just ignore the rest of the frame (00,00 in this case)) "
						mk2.Write(b.Marshal())
					}
				default:
					slog.Error("unhandled frame received", slog.Any("frame", hex.EncodeToString(frame)))
				}
			case <-time.After(time.Second * 2):
				b := vebus.Command('V').Frame('$', 0xdb, 0x11, 0x00, 0x00)
				mk2.Write(b.Marshal())
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	mk2.Shutdown()
}
