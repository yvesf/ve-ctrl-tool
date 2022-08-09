package victron

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"ve-ctrl-tool/victron/veBus"
)

type Mk2 struct {
	*mk2IO
}

func NewMk2(address string) (*Mk2, error) {
	reader, err := NewReader(address)
	if err != nil {
		return nil, err
	}
	return &Mk2{reader}, nil
}

// SetAddress selects VE.Bus device at 'address'
func (m Mk2) SetAddress(ctx context.Context, address byte) error {
	log.Debug().Msgf("SetAddress 0x%x", address)
	// 0x01 means "set"
	frame, err := veBus.CommandA.Frame(0x01, address).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to select address: %w", err)
	}
	if frame.Data[0] != 0x01 {
		return fmt.Errorf("return action %v is not 0x01", frame.Data[0])
	}
	if frame.Data[1] != address {
		return fmt.Errorf("return address %v is not the requested one %v", frame.Data[1], address)
	}
	log.Debug().Msgf("SetAddress selected 0x%x", address)

	return nil
}

func (m Mk2) GetAddress(ctx context.Context) (byte, error) {
	log.Debug().Msg("GetAddress")
	// 0x00 means "not set"
	frame, err := veBus.CommandA.Frame(0x00 /*ignored:*/, 0x00).WriteAndRead(ctx, m)
	if err != nil {
		return 0, fmt.Errorf("failed to select address: %w", err)
	}
	log.Debug().Msgf("GetAddress selected 0x%x", frame.Data[0])

	return frame.Data[0], nil
}

// CommandSendSoftwareVersionPart0 is used to read the state of the device or to force the unit to go into a specific state.
//func (m Mk2) CommandSendSoftwareVersionPart0(ctx context.Context) (int, error) {
//	log.Debug().Msg("CommandSendSoftwareVersionPart0 request")
//	frame, err := writeAndRead(ctx, m.mk2IO, victron.WCommandSendSoftwareVersionPart0.Frame(0x00, 0x00))
//	if err != nil {
//		return 0, fmt.Errorf("failed to execute CommandSendSoftwareVersion: %w", err)
//	}
//	if frame.Command != 0x82 {
//		return 0, fmt.Errorf("wrong response command, got 0x%x, expected 0x%x", frame.Command, 0x82)
//	}
//	versionPart0 := int(frame[4]) + int(frame[5])<<8
//	log.Info().Msgf("CommandSendSoftwareVersionPart0 response versionPart0 = %d", versionPart0)
//	return versionPart0, nil
//}

// CommandSendSoftwareVersionPart1 is used to read the state of the device or to force the unit to go into a specific state.
//func (m Mk2) CommandSendSoftwareVersionPart1(ctx context.Context) (int, error) {
//	log.Debug().Msg("CommandSendSoftwareVersionPart1 request")
//	frame, err := m.WriteAndReadFrame(ctx, victron.WCommandSendSoftwareVersionPart1.Frame(0x00, 0x00))
//	if err != nil {
//		return 0, fmt.Errorf("failed to execute CommandSendSoftwareVersion: %w", err)
//	}
//	if frame[3] != 0x83 {
//		return 0, fmt.Errorf("wrong response command, got 0x%x, expected 0x%x", frame[3], 0x82)
//	}
//	versionPart0 := int(frame[4]) + int(frame[5])<<8
//	log.Info().Msgf("CommandSendSoftwareVersionPart1 response versionPart1 = %d", versionPart0)
//	return versionPart0, nil
//}

type DeviceStateRequestState byte

//nolint:deadcode
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
	frame, err := veBus.WCommandGetSetDeviceState.Frame(byte(setState), 0x00).WriteAndRead(ctx, m)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute CommandGetSetDeviceState: %w", err)
	}
	if frame.Reply != veBus.WReplyCommandGetSetDeviceStateOK {
		return "", "", fmt.Errorf("invalid response code to CommandGetSetDeviceState")
	}
	if len(frame.Data) < 2 {
		return "", "", fmt.Errorf("invalid response length to CommandGetSetDeviceState")
	}

	if s, ok := DeviceStateResponseStates[int(frame.Data[0])]; ok {
		state = s
	} else {
		state = DeviceStateResponseStates[-1]
	}
	if s, ok := DeviceStateResponseSubStates[int(frame.Data[1])]; ok {
		subState = s
	} else {
		subState = DeviceStateResponseSubStates[-1]
	}

	log.Debug().Msgf("CommandGetSetDeviceState state=%v subState=%v", state, subState)

	return state, subState, nil
}

var ErrSettingNotSupported = errors.New("SETTING_NOT_SUPPORTED")

func (m Mk2) CommandReadSetting(ctx context.Context, lowSettingID, highSettingID byte) (lowValue, highValue byte, err error) {
	frame, err := veBus.WCommandReadSetting.Frame(lowSettingID, highSettingID).WriteAndRead(ctx, m)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute CommandGetSetDeviceState: %w", err)
	}
	switch frame.Reply {
	case veBus.WReplySettingNotSupported:
		return 0, 0, ErrSettingNotSupported
	case veBus.WReplyReadSettingOK:
	default:
		return 0, 0, fmt.Errorf("unknown response: %v", frame.Reply.String())
	}

	if len(frame.Data) != 2 {
		return 0, 0, fmt.Errorf("invalid response length to CommandReadSetting")
	}

	return frame.Data[0], frame.Data[1], nil
}

var ErrVariableNotSupported = errors.New("VARIABLE_NOT_SUPPORTED")

func (m Mk2) CommandReadRAMVar(ctx context.Context, ramId0, ramId1 byte) (value0Low, value0High, value1Low, value1High byte, err error) {
	frame, err := veBus.WCommandReadRAMVar.Frame(ramId0, ramId1).WriteAndRead(ctx, m)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to execute CommandReadRAMVar: %w", err)
	}

	switch frame.Reply {
	case veBus.WReplyVariableNotSupported:
		return 0, 0, 0, 0, ErrVariableNotSupported
	case veBus.WReplyReadRAMOK:
		break
	default:
		return 0, 0, 0, 0, fmt.Errorf("unknown response: %v", frame.Reply.String())
	}

	if len(frame.Data) != 4 && len(frame.Data) != 6 {
		// Old devices send 4, newer support requesting two ram-ids at the same time.
		// we drop the second as we asked for 0x00 (UMains) but don't really care about it.
		return 0, 0, 0, 0, fmt.Errorf("invalid response length to CommandReadRAMVar")
	}

	return frame.Data[0], frame.Data[1], frame.Data[2], frame.Data[3], nil
}

func (m Mk2) CommandReadRAMVarUnsigned16(ctx context.Context, ramId0, ramId1 byte) (value0, value1 uint16, err error) {
	v0l, v0h, v1l, v1h, err := m.CommandReadRAMVar(ctx, ramId0, ramId1)
	if err != nil {
		return 0, 0, err
	}
	return uint16(v0l) | uint16(v0h)<<8, uint16(v1l) | uint16(v1h)<<8, nil
}

func (m Mk2) CommandReadRAMVarSigned16(ctx context.Context, ramId0, ramId1 byte) (value0, value1 int16, err error) {
	v0l, v0h, v1l, v1h, err := m.CommandReadRAMVar(ctx, ramId0, ramId1)
	if err != nil {
		return 0, 0, err
	}
	return veBus.ParseSigned16Bytes(v0l, v0h), veBus.ParseSigned16Bytes(v1l, v1h), nil
}

func (m Mk2) CommandWriteRAMVarDataSigned(ctx context.Context, ram uint16, value int16) error {
	low, high := veBus.Signed16Bytes(value)
	return m.CommandWriteRAMVarData(ctx, ram, low, high)
}

func (m Mk2) CommandWriteRAMVarData(ctx context.Context, ram uint16, low, high byte) error {
	m.Write(veBus.WCommandWriteRAMVar.Frame(byte(ram&0xff), byte(ram>>8)).Marshal()) // no response
	frame, err := veBus.WCommandWriteData.Frame(low, high).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteRAMVarData: %w", err)
	}
	switch frame.Reply {
	case veBus.WReplySuccesfulRAMWrite:
		return nil
	default:
		return fmt.Errorf("unknown response: %v", frame.Reply.String())
	}
}

func (m Mk2) CommandWriteViaID(ctx context.Context, id byte, dataLow, dataHigh byte) error {
	// [1]: true => ram value only, false => ram and eeprom
	// [0]: true: setting, false: ram var
	//var flags = byte(0b00000010)
	var flags = byte(0b00000000)
	frame, err := veBus.WCommandWriteViaID.Frame(flags, id, dataLow, dataHigh).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteViaID: %w", err)
	}
	switch frame.Reply {
	case veBus.WReplySuccesfulRAMWrite, veBus.WReplySuccesfulSettingWrite:
		return nil
	default:
		return fmt.Errorf("unknown response: %v", frame.Reply.String())
	}
}

func (m Mk2) CommandWriteSettingData(ctx context.Context, setting uint16, dataLow, dataHigh byte) error {
	m.Write(veBus.WCommandWriteSetting.Frame(byte(setting&0xff), byte(setting>>8)).Marshal()) // no response
	frame, err := veBus.WCommandWriteData.Frame(dataLow, dataHigh).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteSettingData:: %w", err)
	}
	if frame.Reply != veBus.WReplySuccesfulSettingWrite {
		return fmt.Errorf("write failed")
	}
	return nil
}
