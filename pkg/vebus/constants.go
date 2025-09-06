package vebus

import "fmt"

// The following block defines RAM IDs according to "Interfacing with VE Bus products - MK2 Protocol 3 14.docx".
const (
	RAMIDUMainsRMS                 RAMID = 0
	RAMIDIMainsRMS                 RAMID = 1
	RAMIDUInverterRMS              RAMID = 2
	RAMIDIINverterRMS              RAMID = 3
	RAMIDUBat                      RAMID = 4
	RAMIDIBat                      RAMID = 5
	RAMIDUBatRMS                   RAMID = 6 // RMS=value of ripple voltage
	RAMIDInverterPeriodTime        RAMID = 7 // time-base 0.1s
	RAMIDMainsPeriodTime           RAMID = 8 // time-base 0.1s
	RAMIDSignedACLoadCurrent       RAMID = 9
	RAMIDVirtualSwitchPosition     RAMID = 10
	RAMIDIgnoreACInputState        RAMID = 11
	RAMIDMultiFunctionalRelayState RAMID = 12
	RAMIDChargeState               RAMID = 13 // battery monitor function
	RAMIDInverterPower1            RAMID = 14 // filtered. 16bit signed integer. Positive AC->DC. Negative DC->AC.
	RAMIDInverterPower2            RAMID = 15 // ..
	RAMIDOutputPower               RAMID = 16 // AC Output. 16bit signed integer.
	RAMIDInverterPower1Unfiltered  RAMID = 17
	RAMIDInverterPower2Unfiltered  RAMID = 18
	RAMIDOutputPowerUnfiltered     RAMID = 19
)

func (r RAMID) String() string {
	switch r {
	case RAMIDUMainsRMS:
		return fmt.Sprintf("RAMIDUMainsRMS(%d)", r)
	case RAMIDIMainsRMS:
		return fmt.Sprintf("RAMIDIMainsRMS(%d)", r)
	case RAMIDUInverterRMS:
		return fmt.Sprintf("RAMIDUInverterRMS(%d)", r)
	case RAMIDIINverterRMS:
		return fmt.Sprintf("RAMIDIINverterRMS(%d)", r)
	case RAMIDUBat:
		return fmt.Sprintf("RAMIDUBat(%d)", r)
	case RAMIDIBat:
		return fmt.Sprintf("RAMIDIBat(%d)", r)
	case RAMIDUBatRMS:
		return fmt.Sprintf("RAMIDUBatRMS(%d)", r)
	case RAMIDInverterPeriodTime:
		return fmt.Sprintf("RAMIDInverterPeriodTime(%d)", r)
	case RAMIDMainsPeriodTime:
		return fmt.Sprintf("RAMIDMainsPeriodTime(%d)", r)
	case RAMIDSignedACLoadCurrent:
		return fmt.Sprintf("RAMIDSignedACLoadCurrent(%d)", r)
	case RAMIDVirtualSwitchPosition:
		return fmt.Sprintf("RAMIDVirtualSwitchPosition(%d)", r)
	case RAMIDIgnoreACInputState:
		return fmt.Sprintf("RAMIDIgnoreACInputState(%d)", r)
	case RAMIDMultiFunctionalRelayState:
		return fmt.Sprintf("RAMIDMultiFunctionalRelayState(%d)", r)
	case RAMIDChargeState:
		return fmt.Sprintf("RAMIDChargeState(%d)", r)
	case RAMIDInverterPower1:
		return fmt.Sprintf("RAMIDInverterPower1(%d)", r)
	case RAMIDInverterPower2:
		return fmt.Sprintf("RAMIDInverterPower2(%d)", r)
	case RAMIDOutputPower:
		return fmt.Sprintf("RAMIDOutputPower(%d)", r)
	case RAMIDInverterPower1Unfiltered:
		return fmt.Sprintf("RAMIDInverterPower1Unfiltered(%d)", r)
	case RAMIDInverterPower2Unfiltered:
		return fmt.Sprintf("RAMIDInverterPower2Unfiltered(%d)", r)
	case RAMIDOutputPowerUnfiltered:
		return fmt.Sprintf("RAMIDOutputPowerUnfiltered(%d)", r)
	default:
		return fmt.Sprintf("Unknown(%d)", r)
	}
}

// The following block defines Assistent ID to identify to which
// assistant RAM records belong to.
const (
	AssistantRAMIDESS = 5 // ESS Assistant ID.
)

type Command byte

func (c Command) Frame(data ...byte) VeCommandFrame {
	return VeCommandFrame{
		command: c,
		Data:    data,
	}
}

const (
	// CommandA selects the address of the device to communicate with.
	CommandA Command = 'A'
	CommandN Command = 'N'
	// CommandW prefixes the so called "Winmon" commands.
	// They are a whole family of subcommands implemented as WCommand and WReply.
	CommandW Command = 'W'
	CommandR Command = 'R'
	// CommandS ‘S’ (state) command can be used to send the panel state (switch position and input
	// current set point) to the VE.Bus system.
	CommandS Command = 'S'
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

type SettingID uint16

const (
	SettingIDFlags0                       SettingID = 0
	SettingIDFlags1                       SettingID = 1
	SettingIDUBatAbsorption               SettingID = 2
	SettingIDUBatFloat                    SettingID = 3
	SettingIDIBatBulk                     SettingID = 4
	SettingIDUInvSetpoint                 SettingID = 5
	SettingIDIMainsLimitAC1               SettingID = 6
	SettingIDRepeatedAbsorptionTime       SettingID = 7
	SettingIDRepeatedAbsorptionInterval   SettingID = 8
	SettingIDMaximumAbsorptionDuration    SettingID = 9
	SettingIDChargecharacteristic         SettingID = 10
	SettingIDUBatLowLimitForInverter      SettingID = 11
	SettingIDUBatLowhysteresisforInverter SettingID = 12
	SettingIDBatteryCapacity              SettingID = 64
)

func (s SettingID) String() string {
	switch s {
	case SettingIDFlags0:
		return "SettingIDFlags0"
	case SettingIDFlags1:
		return "SettingIDFlags1"
	case SettingIDUBatAbsorption:
		return "SettingIDUBatAbsorption"
	case SettingIDUBatFloat:
		return "SettingIDUBatFloat"
	case SettingIDIBatBulk:
		return "SettingIDIBatBulk"
	case SettingIDUInvSetpoint:
		return "SettingIDUInvSetpoint"
	case SettingIDIMainsLimitAC1:
		return "SettingIDIMainsLimitAC1"
	case SettingIDRepeatedAbsorptionTime:
		return "SettingIDRepeatedAbsorptionTime"
	case SettingIDRepeatedAbsorptionInterval:
		return "SettingIDRepeatedAbsorptionInterval"
	case SettingIDMaximumAbsorptionDuration:
		return "SettingIDMaximumAbsorptionDuration"
	case SettingIDChargecharacteristic:
		return "SettingIDChargecharacteristic"
	case SettingIDUBatLowLimitForInverter:
		return "SettingIDUBatLowLimitForInverter"
	case SettingIDUBatLowhysteresisforInverter:
		return "SettingIDUBatLowhysteresisforInverter"
	case SettingIDBatteryCapacity:
		return "SettingIDBatteryCapacity"
	default:
		return fmt.Sprintf("undefined setting 0x%04x", uint16(s))
	}
}
