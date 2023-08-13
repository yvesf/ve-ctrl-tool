package vebus

import (
	"context"
)

// Signed16Bytes is nothing special but just literally implements the signed-integer encoding in ESS Mode 2 and 3.
func Signed16Bytes(in int16) (uint8, uint8) {
	var val uint16
	if in < 0 {
		val = 1 + ^(uint16(in * -1))
	} else {
		val = uint16(in)
	}
	return uint8(0xff & val), uint8(0xff & (val >> 8))
}

// ParseSigned16 is nothing special but just literally implements the signed-integer encoding in ESS Mode 2 and 3.
func ParseSigned16(in uint16) int16 {
	if 0x8000&in == 0x8000 {
		return -1 + int16(0x7fff & ^in)*-1
	}
	return int16(in)
}

func ParseSigned16Bytes(low, high byte) int16 {
	return ParseSigned16(uint16(low) | uint16(high)<<8)
}

// Checksum implements the check-summing algorithm for ve.bus.
func Checksum(data []byte) byte {
	checksum := byte(0)
	for _, d := range data {
		checksum -= d
	}
	return checksum
}

type VeCommandFrame struct {
	command Command
	Data    []byte
}

func (f VeCommandFrame) Marshal() []byte {
	if len(f.Data) > 253 {
		panic("invalid data length")
	}
	length := len(f.Data) + 1 + 1
	result := append([]byte{byte(length), 0xff, byte(f.command)}, f.Data...)
	chksum := Checksum(result)
	result = append(result, chksum)
	return result
}

func (f VeCommandFrame) ParseResponse(data []byte) *VeCommandFrame {
	if len(data) >= 3 {
		if data[2] == byte(f.command) {
			return &VeCommandFrame{
				command: Command(data[2]),
				Data:    data[3:],
			}
		}
	}
	return nil
}

func (f VeCommandFrame) WriteAndRead(ctx context.Context, io frameReadWriter) (response *VeCommandFrame, err error) {
	_, err = io.ReadAndWrite(ctx, f.Marshal(), func(d []byte) bool {
		response = f.ParseResponse(d)
		return response != nil
	})
	return response, err
}

type VeWFrame struct {
	Command WCommand
	Data    []byte
}

func (f VeWFrame) Marshal() []byte {
	return VeCommandFrame{
		command: CommandW,
		Data:    append([]byte{byte(f.Command)}, f.Data...),
	}.Marshal()
}

func (f VeWFrame) ParseResponse(data []byte) *VeWFrameReply {
	if len(data) >= 4 && data[2] == byte(CommandW) {
		return &VeWFrameReply{
			Reply: WReply(data[3]),
			Data:  data[4:],
		}
	}
	return nil
}

func (f VeWFrame) WriteAndRead(ctx context.Context, io frameReadWriter) (response *VeWFrameReply, err error) {
	_, err = io.ReadAndWrite(ctx, f.Marshal(), func(d []byte) bool {
		response = f.ParseResponse(d)
		return response != nil
	})
	return response, err
}

type VeWFrameReply struct {
	Reply WReply
	Data  []byte
}

type VeFrame interface {
	Marshal() []byte
	ParseResponse([]byte) VeFrame
}

type frameReadWriter interface {
	ReadAndWrite(context.Context, []byte, func([]byte) bool) ([]byte, error)
}
