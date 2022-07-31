package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
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
			help:    "read-ram <ramid-byte> (CommandReadRAMVar)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				if len(args) != 1 {
					return fmt.Errorf("wrong no of args")
				}
				ramID, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("failed to parse ramid: %w", err)
				}
				value, err := mk2.CommandReadRAMVar(ctx, byte(ramID))
				if err != nil {
					return fmt.Errorf("read-ram command failed: %w", err)
				}
				fmt.Printf("value=%d value(signed)=%d value=0b%b value=0x%x\n", value, int16(value), value, value)
				return nil
			},
		},
		{
			command: "write-ram",
			args:    3,
			help:    "write-ram <ram-id> <low-byte> <high-byte> (CommandWriteRAMVarData)",
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
				err = mk2.CommandWriteRAMVarData(ctx, uint16(ramID), byte(low), byte(high))
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
				UInverterRMS, err := mk2.CommandReadRAMVar(ctx, byte(2))
				if err != nil {
					return fmt.Errorf("voltage access UInverterRMS failed: %w", err)
				}
				fmt.Printf("UInverterRMS=%d UInverterRMS=0x%x UInverterRMS=0b%b\n", UInverterRMS, UInverterRMS, UInverterRMS)

				IInverterRMS, err := mk2.CommandReadRAMVar(ctx, byte(3))
				if err != nil {
					return fmt.Errorf("voltage access IInverterRMS failed: %w", err)
				}
				fmt.Printf("IInverterRMS=%d IInverterRMS=0x%x IInverterRMS=0b%b\n", IInverterRMS, IInverterRMS, IInverterRMS)

				InverterPower14, err := mk2.CommandReadRAMVar(ctx, byte(14))
				if err != nil {
					return fmt.Errorf("voltage access InverterPower14 failed: %w", err)
				}
				fmt.Printf("InverterPower14=%d InverterPower14=0x%x InverterPower14=0b%b\n", InverterPower14, InverterPower14, InverterPower14)

				OutputPower, err := mk2.CommandReadRAMVar(ctx, byte(16))
				if err != nil {
					return fmt.Errorf("voltage access OutputPower failed: %w", err)
				}
				fmt.Printf("OutputPower=%d OutputPower=0x%x OutputPower=0b%b\n", OutputPower, OutputPower, OutputPower)

				return nil
			},
		},
		{
			command: "set-assist",
			args:    1,
			help:    "set-assist <ampere-int16> (\"S\" command)",
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				if len(args) != 1 {
					return fmt.Errorf("wrong number of args")
				}
				ampere, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse high-byte failed: %w", err)
				}
				err = mk2.SetAssist(ctx, int16(ampere))
				if err != nil {
					return fmt.Errorf("set-assist failed: %w", err)
				}
				return nil
			},
		},
		{
			command: `test`,
			args:    0,
			help:    ``,
			fun: func(ctx context.Context, mk2 Mk2, args ...string) error {
				mk2.Write(transportFrame{data: []byte{0x05, 0xff, 0x57, 0x32, 0x81, 0x00}})
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
