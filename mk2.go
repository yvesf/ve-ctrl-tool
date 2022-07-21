package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
)

type Mk2 struct {
	*mk2IO
}

// SetAddress selects VE.Bus device at 'address'
func (m Mk2) SetAddress(ctx context.Context, address byte) error {
	log.Debug().Msgf("SetAddress 0x%x", address)
	frame, err := m.WriteAndReadFrame(ctx, 'A', 0x01, address)
	if err != nil {
		return fmt.Errorf("failed to select address: %w", err)
	}
	if frame.data[4] != address {
		return fmt.Errorf("return address %v is not the requested one %v", frame.data[3], address)
	}
	log.Debug().Msgf("SetAddress selected 0x%x", address)

	return nil
}

func (m Mk2) GetAddress(ctx context.Context) (byte, error) {
	log.Debug().Msg("GetAddress")
	frame, err := m.WriteAndReadFrame(ctx, 'A', 0x01 /*ignored:*/, 0x00)
	if err != nil {
		return 0, fmt.Errorf("failed to select address: %w", err)
	}
	log.Debug().Msgf("GetAddress selected 0x%x", frame.data[3])

	return frame.data[3], nil
}

// CommandSendSoftwareVersionPart0 is used to read the state of the device or to force the unit to go into a specific state.
func (m Mk2) CommandSendSoftwareVersionPart0(ctx context.Context) (int, error) {
	log.Debug().Msg("CommandSendSoftwareVersionPart0 request")
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x05, 0x00, 0x00)
	if err != nil {
		return 0, fmt.Errorf("failed to execute CommandSendSoftwareVersion: %w", err)
	}
	if frame.data[3] != 0x82 {
		return 0, fmt.Errorf("wrong response command, got 0x%x, expected 0x%x", frame.data[3], 0x82)
	}
	versionPart0 := int(frame.data[4]) + int(frame.data[5])<<8
	log.Info().Msgf("CommandSendSoftwareVersionPart0 response versionPart0 = %d", versionPart0)
	return versionPart0, nil
}

// CommandSendSoftwareVersionPart1 is used to read the state of the device or to force the unit to go into a specific state.
func (m Mk2) CommandSendSoftwareVersionPart1(ctx context.Context) (int, error) {
	log.Debug().Msg("CommandSendSoftwareVersionPart1 request")
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x06, 0x00, 0x00)
	if err != nil {
		return 0, fmt.Errorf("failed to execute CommandSendSoftwareVersion: %w", err)
	}
	if frame.data[3] != 0x83 {
		return 0, fmt.Errorf("wrong response command, got 0x%x, expected 0x%x", frame.data[3], 0x82)
	}
	versionPart0 := int(frame.data[4]) + int(frame.data[5])<<8
	log.Info().Msgf("CommandSendSoftwareVersionPart1 response versionPart1 = %d", versionPart0)
	return versionPart0, nil
}

type DeviceStateRequestState byte

const (
	DeviceStateRequestStateInquiry           = 0x0
	DeviceStateRequestStateForceToEqualise   = 0x1
	DeviceStateRequestStateForceToAbsorption = 0x2
	DeviceStateRequestStateForceToFloat      = 0x3
)

type DeviceStateResponseState string

var DeviceStateResponseStates = map[int]DeviceStateResponseState{
	-1:   "<invalid-state>",
	0x00: "down",
	0x01: "startup",
	0x02: "off",
	0x03: "slave-mode",
	0x04: "invert-full",
	0x05: "invert-half",
	0x06: "invert-aes",
	0x07: "power-assist",
	0x08: "bypass",
	0x09: "charge",
}

type DeviceStateResponseSubState string

var DeviceStateResponseSubStates = map[int]DeviceStateResponseSubState{
	-1:   "<invalid-sub-state>",
	0x00: "init",
	0x01: "bulk",
	0x02: "absorption",
	0x03: "float",
	0x04: "storage",
	0x05: "repeated-absorption",
	0x06: "forced-absorption",
	0x07: "equalise",
	0x08: "bulk-stopped",
}

// CommandGetSetDeviceState is used to read the state of the device or to force the unit to go into a specific state.
// Passing 'state' 0 means no change, just reading the current state.
func (m Mk2) CommandGetSetDeviceState(ctx context.Context, setState DeviceStateRequestState) (state DeviceStateResponseState, subState DeviceStateResponseSubState, err error) {
	log.Debug().Msgf("CommandGetSetDeviceState setState=0x%x", setState)
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x0e, byte(setState), 0x00)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute CommandGetSetDeviceState: %w", err)
	}
	if frame.data[3] != 0x94 {
		return "", "", fmt.Errorf("invalid response code to CommandGetSetDeviceState")
	}
	if len(frame.data) < 5 {
		return "", "", fmt.Errorf("invalid response length to CommandGetSetDeviceState")
	}

	if s, ok := DeviceStateResponseStates[int(frame.data[4])]; ok {
		state = s
	} else {
		state = DeviceStateResponseStates[-1]
	}
	if s, ok := DeviceStateResponseSubStates[int(frame.data[5])]; ok {
		subState = s
	} else {
		subState = DeviceStateResponseSubStates[-1]
	}

	log.Debug().Msgf("CommandGetSetDeviceState state=%v subState=%v", state, subState)

	return state, subState, nil
}

var ErrSettingNotSupported = errors.New("SETTING_NOT_SUPPORTED")

func (m Mk2) CommandReadSetting(ctx context.Context, lowSettingID, highSettingID byte) (lowValue, highValue byte, err error) {
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x31, lowSettingID, highSettingID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute CommandGetSetDeviceState: %w", err)
	}
	if frame.data[3] == 0x91 {
		return 0, 0, ErrSettingNotSupported
	}
	if frame.data[3] != 0x86 {
		return 0, 0, fmt.Errorf("invalid response code to CommandReadSetting")
	}

	if len(frame.data) != 2+4 {
		return 0, 0, fmt.Errorf("invalid response length to CommandReadSetting")
	}

	return frame.data[2+2], frame.data[2+3], nil
}

var ErrVariableNotSupported = errors.New("VARIABLE_NOT_SUPPORTED")

func (m Mk2) CommandReadRAMVar(ctx context.Context, ramId byte) (value uint16, err error) {
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x30, ramId)
	if err != nil {
		return 0, fmt.Errorf("failed to execute CommandReadRAMVar: %w", err)
	}
	if frame.data[3] == 0x90 {
		return 0, ErrVariableNotSupported
	}

	if frame.data[3] != 0x85 {
		return 0, fmt.Errorf("invalid response code to CommandReadRAMVar")
	}

	if len(frame.data) != 2+4 && len(frame.data) != 2+6 {
		// BUG?? We get two times the same value here
		/**
		Please note that you will get an extra ValueB. This is a feature of newer Multi firmware versions.
		Because the IDs range from 0..255 the Hi(ID) field would always be 0.
		Newer Multi firmwares allow you to specify a second ID in this field.
		So in this case ValueB is the value of RAMID 0 because the 0x00 is interpreted as the second ID.
		RAMID 0 corresponds with UMains (This can be found in paragraph 7.3.11 of the 'Interfacing with VE.Bus products' document.)
		So in this example the UMains value is 0x5A52 ⇒ 231.22V NOTE: You will always get a ValueB in the response.
		You can make handy use of this by reading an extra RAMID or you can ignore it if you don’t need it.
		*/
		return 0, fmt.Errorf("invalid response length to CommandReadRAMVar")
	}

	return uint16(frame.data[2+2]) + uint16(frame.data[2+3])<<8, nil
}

func (m Mk2) CommandWriteRAMVarData(ctx context.Context, ram uint16, dataLow, dataHigh byte) error {
	m.Write(transportFrame{data: []byte{'W', 0x32, byte(ram & 0xff), byte(ram >> 8)}}) // no response
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x34, dataLow, dataHigh)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteRAMVarData: %w", err)
	}
	if frame.data[3] != 0x87 {
		return fmt.Errorf("write failed")
	}
	return nil
}

func (m Mk2) CommandWriteSettingData(ctx context.Context, setting uint16, dataLow, dataHigh byte) error {
	m.Write(transportFrame{data: []byte{0x05, 0xff, 'W', 0x33, byte(setting & 0xff), byte(setting >> 8)}}) // no response
	frame, err := m.WriteAndReadFrame(ctx, 'W', 0x34, dataLow, dataHigh)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteSettingData:: %w", err)
	}
	if frame.data[3] != 0x88 {
		return fmt.Errorf("write failed")
	}
	return nil
}

func (m Mk2) SetAssist(ctx context.Context, ampere int16) error {
	frame, err := m.WriteAndReadFrame(ctx, 'S', 0x03, byte(ampere&0xff), byte(ampere>>8), 0x01, 0b10000000)
	if err != nil {
		return fmt.Errorf("failed to execute SetAssist: %w", err)
	}
	if frame.data[2] != 'S' {
		return fmt.Errorf("SetAssist failed")
	}
	return nil
}
