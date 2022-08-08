package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"ve-ctrl-tool/backoff"
	"ve-ctrl-tool/victron"
)

type c struct {
	command string
	args    int
	fun     func(ctx context.Context, mk2 Mk2, args ...string) error
	help    string
}

var commands []c

func init() {
	commands = []c{{
		command: "help",
		args:    0,
		help:    "help display this help",
		fun:     func(context.Context, Mk2, ...string) error { help(); return nil },
	},
		{
			command: "state",
			args:    0,
			help:    "state (CommandGetSetDeviceState)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				state, subState, err := mk2.CommandGetSetDeviceState(ctx, DeviceStateRequestStateInquiry)
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
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				f := transportFrame{data: []byte{0x02, 0xff, 'R'}}
				mk2.Write(f)
				time.Sleep(time.Second * 1)
				println("reset finished")
				return nil
			},
		},
		{
			command: "set-state",
			args:    1,
			help:    "set-state 0|1|2|3 (CommandGetSetDeviceState)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				if len(args) != 1 {
					return fmt.Errorf("wrong number of args")
				}
				setState, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid argument format")
				}
				mainState, subState, err := mk2.CommandGetSetDeviceState(ctx, DeviceStateRequestState(setState))
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
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
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
				lowValue, highValue, err := mk2.CommandReadSetting(ctx, byte(low), byte(high))
				if err != nil {
					return fmt.Errorf("command read-setting failed: %w", err)
				}
				fmt.Printf("value=%d low=0x%x high=0x%x low=0b%b high=0b%b\n", int(lowValue)+int(highValue)<<8, lowValue, highValue, lowValue, highValue)
				return nil
			},
		},
		{
			command: "read-ram",
			args:    1,
			help:    "read-ram <ramid-byte (comma sep)> (CommandReadRAMVar)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
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

				value0, value1, err := mk2.CommandReadRAMVar(ctx, ramIds[0], ramIds[1])
				if err != nil {
					return fmt.Errorf("read-ram command failed: %w", err)
				}
				fmt.Printf("value0=%d value0(signed)=%d value0=0b%b value0=0x%x\n", value0, int16(value0), value0, value0)
				fmt.Printf("value1=%d value1(signed)=%d value1=0b%b value1=0x%x\n", value1, int16(value1), value1, value1)
				return nil
			},
		},
		{
			command: "write-ram",
			args:    3,
			help:    "write-ram <ram-id> <uint16-value) (CommandWriteRAMVarData)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				if len(args) != 3 {
					return fmt.Errorf("wrong number of args")
				}
				ramID, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse ram-id failed: %w", err)
				}
				value, err := strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("parse value failed: %w", err)
				}
				err = mk2.CommandWriteRAMVarData(ctx, uint16(ramID), int16(value))
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
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
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
				err = mk2.CommandWriteViaID(ctx, byte(ramID), byte(low), byte(high))
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
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
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
				err = mk2.CommandWriteSettingData(ctx, uint16(settingID), byte(low), byte(high))
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
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				UInverterRMS, IInverterRMS, err := mk2.CommandReadRAMVar(ctx, victron.RamIDUInverterRMS, victron.RamIDIINverterRMS)
				if err != nil {
					return fmt.Errorf("voltage access UInverterRMS failed: %w", err)
				}
				fmt.Printf("UInverterRMS=%d UInverterRMS=0x%x UInverterRMS=0b%b\n", UInverterRMS, UInverterRMS, UInverterRMS)
				fmt.Printf("IInverterRMS=%d IInverterRMS=0x%x IInverterRMS=0b%b\n", IInverterRMS, IInverterRMS, IInverterRMS)

				InverterPower14, OutputPower, err := mk2.CommandReadRAMVar(ctx, victron.RamIDInverterPower1, victron.RamIDOutputPower)
				if err != nil {
					return fmt.Errorf("voltage access InverterPower14 failed: %w", err)
				}
				fmt.Printf("InverterPower14=%d InverterPower14=0x%x InverterPower14=0b%b\n", InverterPower14, InverterPower14, InverterPower14)
				fmt.Printf("OutputPower=%d OutputPower=0x%x OutputPower=0b%b\n", OutputPower, OutputPower, OutputPower)

				return nil
			},
		},
		{
			command: "set-address",
			args:    1,
			help:    "set-address selects the address (\"A\" command, default 0)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				addr, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse addr failed: %w", err)
				}
				err = mk2.SetAddress(ctx, byte(addr))
				if err != nil {
					return fmt.Errorf("address failed: %w", err)
				}
				return nil
			},
		},
		{
			command: "get-address",
			args:    0,
			help:    "address gets the current address (\"A\" command)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				addr, err := mk2.GetAddress(ctx)
				if err != nil {
					return fmt.Errorf("address failed: %w", err)
				}
				fmt.Printf("address=0x%02x\n", addr)
				return nil
			},
		},
		{
			command: "ess-static",
			args:    1,
			help:    "ess-static <arg> (run loop sending signed value to ram 129)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				b := backoff.NewExponentialBackoff(time.Second, time.Second*25)

				setpointWatt, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse high-byte failed: %w", err)
				}

				fmt.Printf("Press enter to stop\n")
				childCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				go func() {
					var x = []byte{0}
					_, _ = os.Stdin.Read(x)
					cancel()
				}()

				var errors = 0
				for childCtx.Err() == nil {
					err := mk2.CommandWriteRAMVarData(ctx, 129, int16(setpointWatt))
					if err != nil {
						return err
					}
					select {
					case <-childCtx.Done():
					case <-time.After(time.Millisecond * 500):
					}

					var UInverterRMS, IInverterRMS, InverterPower14, OutputPower uint16
					UInverterRMS, IInverterRMS, err = mk2.CommandReadRAMVar(ctx, victron.RamIDUInverterRMS, victron.RamIDIINverterRMS)
					if err != nil {
						log.Error().Msgf("voltage access UInverterRMS failed: %v", err)
						goto error
					}
					InverterPower14, OutputPower, err = mk2.CommandReadRAMVar(ctx, victron.RamIDInverterPower1, victron.RamIDOutputPower)
					if err != nil {
						log.Error().Msgf("voltage access InverterPower14 failed: %v", err)
						goto error
					}
					fmt.Printf("UInverterRMS=%d UInverterRMS=0x%x UInverterRMS=0b%b\n", UInverterRMS, UInverterRMS, UInverterRMS)
					fmt.Printf("IInverterRMS=%d IInverterRMS=0x%x IInverterRMS=0b%b\n", IInverterRMS, IInverterRMS, IInverterRMS)
					fmt.Printf("InverterPower14=%d InverterPower14=0x%x InverterPower14=0b%b\n", InverterPower14, InverterPower14, InverterPower14)
					fmt.Printf("OutputPower=%d OutputPower=0x%x OutputPower=0b%b\n", OutputPower, OutputPower, OutputPower)

					errors = 0
					continue

				error:
					errors++
					sleepDuration, next := b.Next(errors)
					if !next {
						break
					}
					log.Info().Float64("seconds", sleepDuration.Seconds()).Msg("sleep after error")
					select {
					case <-childCtx.Done():
					case <-time.After(sleepDuration):
					}
				}

				log.Info().Msg("reset ESS to 0")
				err = mk2.CommandWriteRAMVarData(context.Background(), victron.RamIDAssistent129, 0)
				if err != nil {
					log.Error().Err(err).Msg("failed to write to RAM 129")
				}

				return nil
			},
		},
		{
			command: "ess-shelly",
			args:    1,
			help:    "ess-shelly <url> (run ess with shelly as control input)",
			fun:     CommandEssShelly,
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
