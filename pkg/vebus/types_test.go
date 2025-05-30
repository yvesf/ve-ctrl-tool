package vebus

import (
	"testing"

	"github.com/carlmjohnson/be"
)

func TestSigned16Bytes(t *testing.T) {
	for _, tc := range []struct {
		name            string
		input           int16
		outLow, outHigh byte
	}{
		{name: "zero", input: 0, outLow: 0, outHigh: 0},
		{name: "+1", input: 1, outLow: 1, outHigh: 0},
		{name: "-1", input: -1, outLow: 0xff, outHigh: 0xff},
		{name: "-200", input: -200, outLow: 0x38, outHigh: 0xff},
		{name: "-32768", input: -32768, outLow: 0x00, outHigh: 0x80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			low, high := Signed16Bytes(tc.input)
			if low != tc.outLow {
				t.Errorf("low expected %v, got %v. expected 0x%02x, got 0x%02x", tc.outLow, low, tc.outLow, low)
			}
			if high != tc.outHigh {
				t.Errorf("high expected %v, got %v. expected 0x%02x, got 0x%02x", tc.outHigh, high, tc.outHigh, high)
			}

			input := ParseSigned16(uint16(high)<<8 | uint16(low))
			if input != tc.input {
				t.Errorf("expected %v, got %v. expected 0x%02x, got 0x%02x", tc.input, input, tc.input, input)
			}
		})
	}
}

func TestChecksum(t *testing.T) {
	be.Equal(t, byte(0xbb), Checksum([]byte{0x04, 0xff, 'A', 0x01, 0x00}))
	be.Equal(t, byte(0xba), Checksum([]byte{0x05, 0xff, 'A', 0x01, 0x00, 0x00}))
	be.Equal(t, byte(0xa0), Checksum([]byte{0x05, 0xff, 'W', 0x05, 0x00, 0x00}))
	be.Equal(t, byte(0x9f), Checksum([]byte{0x05, 0xff, 'W', 0x06, 0x00, 0x00}))
	be.Equal(t, byte(0x6b), Checksum([]byte{0x05, 0xff, 'W', 0x36, 0x04, 0x00}))
	be.Equal(t, byte(0x68), Checksum([]byte{0x05, 0xff, 'W', 0x30, 0x0d, 0x00}))
	be.Equal(t, byte(0xb8), Checksum([]byte{0x03, 0xff, 'F', 0x00}))
	be.Equal(t, byte(0xb3), Checksum([]byte{0x02, 0xff, 'L'}))
	be.Equal(t, byte(0xd2), Checksum([]byte{0x07, 0xff, 'S', 0x03, 0xc0, 0x10, 0x01, 0x01}))
	be.Equal(t, byte(0xac), Checksum([]byte{0x02, 0xff, 'S'}))
	be.Equal(t, byte(0x94), Checksum([]byte{7, 255, 86, 36, 219, 17, 0, 0}))
	be.Equal(t, byte(0x52), Checksum([]byte{7, 255, 86, 36, 219, 17, 0, 66}))
	be.Equal(t, byte(160), Checksum([]byte{5, 255, 87, 5, 0, 0}))
	be.Equal(t, byte(0x94), Checksum([]byte{0x7, 0xff, 'V', '$', 0xdb, 0x11, 0x00, 0x00}))
	be.Equal(t, byte(0x01), Checksum([]byte{0x7, 0xff, 'W', 0x85, 0xff, 0xfe, 0xc6, 0x5a}))
}

func TestVeCommandFrame_Marshall(t *testing.T) {
	be.AllEqual(t, []byte{0x04, 0xff, 'A', 0x01, 0x00, 0xbb},
		VeCommandFrame{command: CommandA, Data: []byte{0x01, 0x00}}.Marshal())
	be.AllEqual(t, []byte{0x05, 0xff, 'W', 0x05, 0x00, 0x00, 0xa0},
		VeCommandFrame{command: CommandW, Data: []byte{0x05, 0x00, 0x00}}.Marshal())
}
