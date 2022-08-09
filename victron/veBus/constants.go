package veBus

import "fmt"

// The following block defines RAM IDs according to "Interfacing with VE Bus products - MK2 Protocol 3 14.docx"
const (
	RamIDUMainsRMS                 = 0
	RamIDIMainsRMS                 = 1
	RamIDUInverterRMS              = 2
	RamIDIINverterRMS              = 3
	RamIDUBat                      = 4
	RamIDIBat                      = 5
	RamIDUBatRMS                   = 6 // RMS=value of ripple voltage
	RamIDInverterPeriodTime        = 7 // time-base 0.1s
	RamIDMainsPeriodTime           = 8 // time-base 0.1s
	RamIDSignedACLoadCurrent       = 9
	RamIDVirtualSwitchPosition     = 10
	RamIDIgnoreACInputState        = 11
	RamIDMultiFunctionalRelayState = 12
	RamIDChargeState               = 13 // battery monitor function
	RamIDInverterPower1            = 14 // filtered. 16bit signed integer. Positive AC->DC. Negative DC->AC.
	RamIDInverterPower2            = 15 // ..
	RamIDOutputPower               = 16 // AC Output. 16bit signed integer.
	RamIDInverterPower1Unfiltered  = 17
	RamIDInverterPower2Unfiltered  = 18
	RamIDOutputPowerUnfiltered     = 19
	RamIDAssistent129              = 129
)

type Command byte

func (c Command) Frame(data ...byte) VeCommandFrame {
	return VeCommandFrame{
		command: c,
		Data:    data,
	}
}

const (
	CommandA Command = 'A'
	CommandW Command = 'W'
	CommandR Command = 'R'
)

type WCommand byte

func (c WCommand) Frame(data ...byte) VeWFrame {
	return VeWFrame{
		Command: c,
		Data:    data,
	}
}

const (
	WCommandSendSoftwareVersionPart0 WCommand = 0x05
	WCommandSendSoftwareVersionPart1 WCommand = 0x06
	WCommandGetSetDeviceState        WCommand = 0x0e
	WCommandReadRAMVar               WCommand = 0x30
	WCommandReadSetting              WCommand = 0x31
	WCommandWriteRAMVar              WCommand = 0x32
	WCommandWriteSetting             WCommand = 0x33
	WCommandWriteData                WCommand = 0x34
	WCommandWriteViaID               WCommand = 0x37
)

type WReply uint8

const (
	WReplyCommandNotSupported        = 0x80
	WReplyReadRAMOK                  = 0x85
	WReplyReadSettingOK              = 0x86
	WReplySuccesfulRAMWrite          = 0x87
	WReplySuccesfulSettingWrite      = 0x88
	WReplyVariableNotSupported       = 0x90
	WReplySettingNotSupported        = 0x91
	WReplyCommandGetSetDeviceStateOK = 0x94
	WReplyAccessLevelRequired        = 0x9b
)

func (r WReply) String() string {
	switch r {
	case WReplyCommandNotSupported:
		return "Command not supported"
	case WReplyReadRAMOK:
		return "Read RAM OK"
	case WReplyReadSettingOK:
		return "Read setting OK"
	case WReplySuccesfulRAMWrite:
		return "Write ramvar OK"
	case WReplySuccesfulSettingWrite:
		return "Write setting OK"
	case WReplyVariableNotSupported:
		return "Variable not supported"
	case WReplySettingNotSupported:
		return "Setting not supported"
	case WReplyAccessLevelRequired:
		return "Access level required"
	default:
		return fmt.Sprintf("undefined reply 0x%02x", uint8(r))
	}
}
