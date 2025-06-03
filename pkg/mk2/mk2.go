package mk2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

// Adapter wraps Mk2IO and adds generic functions to execute commands.
type Adapter struct {
	*IO
}

func NewAdapter(address string) (*Adapter, error) {
	reader, err := NewReader(address)
	if err != nil {
		return nil, err
	}
	return &Adapter{reader}, nil
}

// SetAddress selects VE.Bus device at 'address'.
func (m Adapter) SetAddress(ctx context.Context, address byte) error {
	slog.Debug(fmt.Sprintf("SetAddress 0x%x", address))
	// 0x01 means "set"
	frame, err := vebus.CommandA.Frame(0x01, address).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to select address: %w", err)
	}
	if frame.Data[0] != 0x01 {
		return fmt.Errorf("return action %v is not 0x01", frame.Data[0])
	}
	if frame.Data[1] != address {
		return fmt.Errorf("return address %v is not the requested one %v", frame.Data[1], address)
	}
	slog.Debug(fmt.Sprintf("SetAddress selected 0x%x", address))

	return nil
}

func (m Adapter) GetAddress(ctx context.Context) (byte, error) {
	slog.Debug("GetAddress")
	// 0x00 means "not set"
	frame, err := vebus.CommandA.Frame(0x00 /*ignored:*/, 0x00).WriteAndRead(ctx, m)
	if err != nil {
		return 0, fmt.Errorf("failed to select address: %w", err)
	}
	slog.Debug(fmt.Sprintf("GetAddress selected 0x%x", frame.Data[0]))

	return frame.Data[0], nil
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
func (m Adapter) CommandGetSetDeviceState(ctx context.Context, setState DeviceStateRequestState,
) (state DeviceStateResponseState, subState DeviceStateResponseSubState, err error) {
	slog.Debug(fmt.Sprintf("CommandGetSetDeviceState setState=0x%x", setState))
	frame, err := vebus.WCommandGetSetDeviceState.Frame(byte(setState), 0x00).WriteAndRead(ctx, m)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute CommandGetSetDeviceState: %w", err)
	}
	if frame.Reply != vebus.WReplyCommandGetSetDeviceStateOK {
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

	slog.Debug(fmt.Sprintf("CommandGetSetDeviceState state=%v subState=%v", state, subState))

	return state, subState, nil
}

var ErrSettingNotSupported = errors.New("SETTING_NOT_SUPPORTED")

func (m Adapter) CommandReadSetting(ctx context.Context, lowSettingID, highSettingID byte,
) (lowValue, highValue byte, err error) {
	frame, err := vebus.WCommandReadSetting.Frame(lowSettingID, highSettingID).WriteAndRead(ctx, m)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute CommandGetSetDeviceState: %w", err)
	}
	switch frame.Reply {
	case vebus.WReplySettingNotSupported:
		return 0, 0, ErrSettingNotSupported
	case vebus.WReplyReadSettingOK:
	default:
		return 0, 0, fmt.Errorf("unknown response: %v", frame.Reply.String())
	}

	if len(frame.Data) != 2 {
		return 0, 0, fmt.Errorf("invalid response length to CommandReadSetting")
	}

	return frame.Data[0], frame.Data[1], nil
}

var ErrVariableNotSupported = errors.New("VARIABLE_NOT_SUPPORTED")

func (m Adapter) CommandReadRAMVar(ctx context.Context, ramID0, ramID1 byte,
) (value0Low, value0High, value1Low, value1High byte, err error) {
	frame, err := vebus.WCommandReadRAMVar.Frame(ramID0, ramID1).WriteAndRead(ctx, m)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to execute CommandReadRAMVar: %w", err)
	}

	switch frame.Reply {
	case vebus.WReplyVariableNotSupported:
		return 0, 0, 0, 0, ErrVariableNotSupported
	case vebus.WReplyReadRAMOK:
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

func (m Adapter) CommandReadRAMVarUnsigned16(ctx context.Context, ramID0, ramID1 byte,
) (value0, value1 uint16, err error) {
	v0l, v0h, v1l, v1h, err := m.CommandReadRAMVar(ctx, ramID0, ramID1)
	if err != nil {
		return 0, 0, err
	}
	return uint16(v0l) | uint16(v0h)<<8, uint16(v1l) | uint16(v1h)<<8, nil
}

func (m Adapter) CommandReadRAMVarSigned16(ctx context.Context, ramID0, ramID1 byte,
) (value0, value1 int16, err error) {
	v0l, v0h, v1l, v1h, err := m.CommandReadRAMVar(ctx, ramID0, ramID1)
	if err != nil {
		return 0, 0, err
	}
	return vebus.ParseSigned16Bytes(v0l, v0h), vebus.ParseSigned16Bytes(v1l, v1h), nil
}

func (m Adapter) CommandWriteRAMVarDataSigned(ctx context.Context, ram uint16, value int16) error {
	low, high := vebus.Signed16Bytes(value)
	return m.CommandWriteRAMVarData(ctx, ram, low, high)
}

func (m Adapter) CommandWriteRAMVarData(ctx context.Context, ram uint16, low, high byte) error {
	m.Write(vebus.WCommandWriteRAMVar.Frame(byte(ram&0xff), byte(ram>>8)).Marshal()) // no response
	frame, err := vebus.WCommandWriteData.Frame(low, high).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteRAMVarData: %w", err)
	}
	switch frame.Reply {
	case vebus.WReplySuccesfulRAMWrite:
		return nil
	default:
		return fmt.Errorf("unknown response: %v", frame.Reply.String())
	}
}

func (m Adapter) CommandWriteViaID(ctx context.Context, id byte, dataLow, dataHigh byte) error {
	// [1]: true => ram value only, false => ram and eeprom
	// [0]: true: setting, false: ram var
	flags := byte(0b00000000)
	frame, err := vebus.WCommandWriteViaID.Frame(flags, id, dataLow, dataHigh).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteViaID: %w", err)
	}
	switch frame.Reply {
	case vebus.WReplySuccesfulRAMWrite, vebus.WReplySuccesfulSettingWrite:
		return nil
	default:
		return fmt.Errorf("unknown response: %v", frame.Reply.String())
	}
}

func (m Adapter) CommandWriteSettingData(ctx context.Context, setting uint16, dataLow, dataHigh byte) error {
	m.Write(vebus.WCommandWriteSetting.Frame(byte(setting&0xff), byte(setting>>8)).Marshal()) // no response
	frame, err := vebus.WCommandWriteData.Frame(dataLow, dataHigh).WriteAndRead(ctx, m)
	if err != nil {
		return fmt.Errorf("failed to execute CommandWriteSettingData:: %w", err)
	}
	if frame.Reply != vebus.WReplySuccesfulSettingWrite {
		return fmt.Errorf("write failed")
	}
	return nil
}
