package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slog"

	"github.com/yvesf/ve-ctrl-tool/pkg/backoff"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/timemock"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

type c struct {
	command string
	args    int
	fun     func(ctx context.Context, adapter *mk2.Adapter, args ...string) error
	help    string
}

var commands []c

func init() {
	commands = []c{
		{
			command: "help",
			args:    0,
			help:    "help display this help",
			fun:     func(context.Context, *mk2.Adapter, ...string) error { help(); return nil },
		},
		{
			command: "state",
			args:    0,
			help:    "state (CommandGetSetDeviceState)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				state, subState, err := adapter.CommandGetSetDeviceState(ctx, mk2.DeviceStateRequestStateInquiry)
				if err != nil {
					return fmt.Errorf("state command failed: %w", err)
				}
				println("device state", state, subState)

				return nil
			},
		},
		{
			command: "reset",
			args:    0,
			help:    "reset requests sends \"R\" to request a device reset",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				_, _ = vebus.CommandR.Frame().WriteAndRead(ctx, adapter)
				timemock.Sleep(time.Second * 1)
				println("reset finished")
				return nil
			},
		},
		{
			command: "set-state",
			args:    1,
			help:    "set-state 0|1|2|3 (CommandGetSetDeviceState)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				if len(args) != 1 {
					return fmt.Errorf("wrong number of args")
				}
				setState, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid argument format")
				}
				mainState, subState, err := adapter.CommandGetSetDeviceState(ctx, mk2.DeviceStateRequestState(setState))
				if err != nil {
					return fmt.Errorf("command set-state failed: %w", err)
				}
				println("device state", mainState, subState)
				return nil
			},
		},
		{
			command: "read-setting",
			args:    2,
			help:    "read-setting <low-byte> <high-byte> (CommandReadSetting)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				if len(args) != 2 {
					return fmt.Errorf("wrong number of args")
				}
				low, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("low byte")
				}
				high, err := strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("high byte")
				}
				lowValue, highValue, err := adapter.CommandReadSetting(ctx, byte(low), byte(high))
				if err != nil {
					return fmt.Errorf("command read-setting failed: %w", err)
				}
				fmt.Printf("value=%d low=0x%x high=0x%x low=0b%b high=0b%b\n",
					int(lowValue)+int(highValue)<<8, lowValue, highValue, lowValue, highValue)
				return nil
			},
		},
		{
			command: "read-ram",
			args:    1,
			help:    "read-ram <ramid-byte (comma sep)> (CommandReadRAMVar)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				if len(args) != 1 {
					return fmt.Errorf("wrong no of args")
				}
				var ramIds []byte
				for _, arg := range strings.Split(args[0], ",") {
					v, err := strconv.ParseUint(arg, 10, 8)
					if err != nil {
						return fmt.Errorf("failed to parse ramid: %w", err)
					}
					ramIds = append(ramIds, byte(v))
				}
				if len(ramIds) == 1 {
					ramIds = append(ramIds, 0)
				}

				value0, value1, err := adapter.CommandReadRAMVarUnsigned16(ctx, ramIds[0], ramIds[1])
				if err != nil {
					return fmt.Errorf("read-ram command failed: %w", err)
				}
				fmt.Printf("value0=%d value0(signed)=%d value0=0b%b value0=0x%x\n",
					value0, vebus.ParseSigned16(value0), value0, value0)
				fmt.Printf("value1=%d value1(signed)=%d value1=0b%b value1=0x%x\n",
					value1, vebus.ParseSigned16(value1), value1, value1)
				return nil
			},
		},
		{
			command: "write-ram-signed",
			args:    2,
			help:    "write-ram-signed <ram-id> <int16-value) (CommandWriteRAMVarData)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				if len(args) != 2 {
					return fmt.Errorf("wrong number of args")
				}
				ramID, err := strconv.ParseUint(args[0], 10, 16)
				if err != nil {
					return fmt.Errorf("parse ram-id failed: %w", err)
				}
				value, err := strconv.ParseInt(args[1], 10, 16)
				if err != nil {
					return fmt.Errorf("parse value failed: %w", err)
				}
				err = adapter.CommandWriteRAMVarDataSigned(ctx, uint16(ramID), int16(value))
				if err != nil {
					return fmt.Errorf("write-ram failed: %w", err)
				}
				return nil
			},
		},
		{
			command: `write-ram-id`,
			args:    3,
			help:    "write-ram-id <ram-id> <low-byte> <high-byte> (CommandWriteViaID)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				if len(args) != 3 {
					return fmt.Errorf("wrong number of args")
				}
				ramID, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse ram-id failed: %w", err)
				}
				low, err := strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("parse low-byte failed: %w", err)
				}
				high, err := strconv.Atoi(args[2])
				if err != nil {
					return fmt.Errorf("parse high-byte failed: %w", err)
				}
				err = adapter.CommandWriteViaID(ctx, byte(ramID), byte(low), byte(high))
				if err != nil {
					return fmt.Errorf("write-ram failed: %w", err)
				}
				return nil
			},
		},
		{
			command: "write-setting",
			args:    3,
			help:    "write-setting <setting-id-uint16> <low-byte> <high-byte> (CommandWriteSettingData)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				if len(args) != 3 {
					return fmt.Errorf("wrong number of args")
				}
				settingID, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse setting-id failed: %w", err)
				}
				low, err := strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("parse low-byte failed: %w", err)
				}
				high, err := strconv.Atoi(args[2])
				if err != nil {
					return fmt.Errorf("parse high-byte failed: %w", err)
				}
				err = adapter.CommandWriteSettingData(ctx, uint16(settingID), byte(low), byte(high))
				if err != nil {
					return fmt.Errorf("write-setting failed: %w", err)
				}
				return nil
			},
		},
		{
			command: "voltage",
			args:    0,
			help:    "voltage shows voltage information from ram",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				uBat, uInverter, err := adapter.CommandReadRAMVarUnsigned16(ctx, vebus.RAMIDUBat, vebus.RAMIDUInverterRMS)
				if err != nil {
					return fmt.Errorf("voltage access UInverterRMS failed: %w", err)
				}
				fmt.Printf("UBat: %.2f Volt  UInverter: %.2f\n", float32(uBat)/100, float32(uInverter)/100)
				return nil
			},
		},
		{
			command: "set-address",
			args:    1,
			help:    "set-address selects the address (\"A\" command, default 0)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				addr, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse addr failed: %w", err)
				}
				err = adapter.SetAddress(ctx, byte(addr))
				if err != nil {
					return fmt.Errorf("set-address failed: %w", err)
				}
				return nil
			},
		},
		{
			command: "get-address",
			args:    0,
			help:    "get-address gets the current address (\"A\" command)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				addr, err := adapter.GetAddress(ctx)
				if err != nil {
					return fmt.Errorf("get-address failed: %w", err)
				}
				fmt.Printf("address=0x%02x\n", addr)
				return nil
			},
		},
		{
			command: "ess-static",
			args:    1,
			help:    "ess-static <arg> (run loop sending signed value to ESS Ram)",
			fun: func(ctx context.Context, adapter *mk2.Adapter, args ...string) error {
				b := backoff.NewExponentialBackoff(time.Second, time.Second*25)

				setpointWatt, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse high-byte failed: %w", err)
				}

				mk2Ess, err := mk2.ESSInit(ctx, adapter)
				if err != nil {
					return err
				}

				fmt.Printf("Press enter to stop\n")
				childCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				go func() {
					x := []byte{0}
					_, _ = os.Stdin.Read(x)
					cancel()
				}()

				errors := 0
				for childCtx.Err() == nil {
					err := mk2Ess.SetpointSet(ctx, int16(setpointWatt))
					if err != nil {
						return fmt.Errorf("failed to set ESS setpoint: %w", err)
					}
					select {
					case <-childCtx.Done():
					case <-timemock.After(time.Millisecond * 500):
					}

					var UInverterRMS, IInverterRMS uint16
					var InverterPower14, OutputPower int16
					var UBattery, IBattery int16
					UInverterRMS, IInverterRMS, err = adapter.CommandReadRAMVarUnsigned16(ctx,
						vebus.RAMIDUInverterRMS, vebus.RAMIDIINverterRMS)
					if err != nil {
						slog.Error("voltage access UInverterRMS failed", slog.Any("err", err))
						goto handleError
					}
					InverterPower14, OutputPower, err = adapter.CommandReadRAMVarSigned16(ctx,
						vebus.RAMIDInverterPower1, vebus.RAMIDOutputPower)
					if err != nil {
						slog.Error("voltage access InverterPower14 failed", slog.Any("err", err))
						goto handleError
					}
					UBattery, IBattery, err = adapter.CommandReadRAMVarSigned16(ctx, vebus.RAMIDUBatRMS, vebus.RAMIDIBat)
					if err != nil {
						slog.Error("voltage access InverterPower14 failed", slog.Any("err", err))
						goto handleError
					}

					fmt.Printf("UInverterRMS=%.2f V\n", float32(UInverterRMS)/100.0)
					fmt.Printf("IInverterRMS=%.2f A\n", float32(IInverterRMS)/100.0)
					fmt.Printf("InverterPower14=%d W\n", InverterPower14)
					fmt.Printf("OutputPower=%d W\n", OutputPower)
					fmt.Printf("UBatteryRMS=%d V\n", UBattery)
					fmt.Printf("IBattery=%.1f A\n", float32(IBattery)/10.0)
					errors = 0
					continue

				handleError:
					errors++
					sleepDuration, next := b.Next(errors)
					if !next {
						break
					}
					slog.Info("sleep after error", slog.Float64("seconds", sleepDuration.Seconds()))
					select {
					case <-childCtx.Done():
					case <-timemock.After(sleepDuration):
					}
				}

				slog.Info("reset ESS to 0")
				err = mk2Ess.SetpointSet(ctx, 0)
				if err != nil {
					slog.Error("failed to reset ESS to 0", slog.Any("err", err))
				}

				return nil
			},
		},
	}
}

func help() {
	fmt.Printf("CLI flags help:\n")
	flag.PrintDefaults()

	fmt.Printf("\nCommands help:\n")
	for _, c := range commands {
		fmt.Printf("\t%s\n", strings.ReplaceAll(c.help, "\n", "\n\t"))
	}
}
