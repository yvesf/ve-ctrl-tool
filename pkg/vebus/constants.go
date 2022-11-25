package vebus

import "fmt"

// The following block defines RAM IDs according to "Interfacing with VE Bus products - MK2 Protocol 3 14.docx".
const (
	RAMIDUMainsRMS                 = 0
	RAMIDIMainsRMS                 = 1
	RAMIDUInverterRMS              = 2
	RAMIDIINverterRMS              = 3
	RAMIDUBat                      = 4
	RAMIDIBat                      = 5
	RAMIDUBatRMS                   = 6 // RMS=value of ripple voltage
	RAMIDInverterPeriodTime        = 7 // time-base 0.1s
	RAMIDMainsPeriodTime           = 8 // time-base 0.1s
	RAMIDSignedACLoadCurrent       = 9
	RAMIDVirtualSwitchPosition     = 10
	RAMIDIgnoreACInputState        = 11
	RAMIDMultiFunctionalRelayState = 12
	RAMIDChargeState               = 13 // battery monitor function
	RAMIDInverterPower1            = 14 // filtered. 16bit signed integer. Positive AC->DC. Negative DC->AC.
	RAMIDInverterPower2            = 15 // ..
	RAMIDOutputPower               = 16 // AC Output. 16bit signed integer.
	RAMIDInverterPower1Unfiltered  = 17
	RAMIDInverterPower2Unfiltered  = 18
	RAMIDOutputPowerUnfiltered     = 19
)

// The following block defines Assistent ID to identify to which
// assistant RAM records belong to.
const (
	AssistantRAMIDESS = 5 // ESS Assistant
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
