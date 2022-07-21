package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecksum(t *testing.T) {
	assert.Equal(t, byte(0xbb), checksum([]byte{0x04, 0xff, 'A', 0x01, 0x00}))
	assert.Equal(t, byte(0xba), checksum([]byte{0x05, 0xff, 'A', 0x01, 0x00, 0x00}))
	assert.Equal(t, byte(0xa0), checksum([]byte{0x05, 0xff, 'W', 0x05, 0x00, 0x00}))
	assert.Equal(t, byte(0x9f), checksum([]byte{0x05, 0xff, 'W', 0x06, 0x00, 0x00}))
	assert.Equal(t, byte(0x6b), checksum([]byte{0x05, 0xff, 'W', 0x36, 0x04, 0x00}))
	assert.Equal(t, byte(0x68), checksum([]byte{0x05, 0xff, 'W', 0x30, 0x0d, 0x00}))
	assert.Equal(t, byte(0xb8), checksum([]byte{0x03, 0xff, 'F', 0x00}))
	assert.Equal(t, byte(0xb3), checksum([]byte{0x02, 0xff, 'L'}))
	assert.Equal(t, byte(0xd2), checksum([]byte{0x07, 0xff, 'S', 0x03, 0xc0, 0x10, 0x01, 0x01}))
	assert.Equal(t, byte(0xac), checksum([]byte{0x02, 0xff, 'S'}))
	assert.Equal(t, byte(0x94), checksum([]byte{7, 255, 86, 36, 219, 17, 0, 0}))
	assert.Equal(t, byte(0x52), checksum([]byte{7, 255, 86, 36, 219, 17, 0, 66}))

	assert.Equal(t, byte(160), checksum([]byte{5, 255, 87, 5, 0, 0}))
}
