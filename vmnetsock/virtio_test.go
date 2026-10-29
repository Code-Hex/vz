package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestVirtioEthernetPacket(t *testing.T) {
	for _, flags := range []byte{0, 2, 4, 6, 0x80} {
		packet := make([]byte, 12+14)
		packet[0] = flags
		binary.LittleEndian.PutUint16(packet[6:], 0xffff)
		binary.LittleEndian.PutUint16(packet[8:], 0xffff)
		copy(packet[12:], []byte("ethernet frame"))
		got, err := virtioEthernetPacket(packet)
		if err != nil || !bytes.Equal(got, []byte("ethernet frame")) {
			t.Fatalf("flags %d: packet = %x, error = %v", flags, got, err)
		}
	}

	// The checksum field already contains the pseudo-header seed 0x1234.
	// The five checksum bytes sum to 0x1234 + 0x5678 + 0x9a00 = 0x102ac.
	packet := make([]byte, 12+19)
	packet[0] = 1
	binary.LittleEndian.PutUint16(packet[6:], 14)
	copy(packet[12+14:], []byte{0x12, 0x34, 0x56, 0x78, 0x9a})
	got, err := virtioEthernetPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[14:], []byte{0xfd, 0x52, 0x56, 0x78, 0x9a}) {
		t.Fatalf("checksum completion = %x", got[14:])
	}

	packet = make([]byte, 12+18)
	packet[0] = 1
	binary.LittleEndian.PutUint16(packet[6:], 14)
	binary.LittleEndian.PutUint16(packet[8:], 2)
	copy(packet[12+14:], []byte{0x12, 0x34, 0xed, 0xcb})
	got, err = virtioEthernetPacket(packet)
	if err != nil || !bytes.Equal(got[14:], []byte{0x12, 0x34, 0xff, 0xff}) {
		t.Fatalf("zero checksum = %x, error = %v", got, err)
	}
}

func TestVirtioEthernetPacketRejectsInvalidMetadata(t *testing.T) {
	for _, test := range []struct {
		name   string
		header []byte
		size   int
	}{
		{name: "short header", size: 11},
		{name: "short Ethernet frame", size: 25},
		{name: "RSC with incomplete checksum", header: []byte{5}, size: 26},
		{name: "GSO", header: []byte{1, 1}, size: 26},
		{name: "checksum start past frame", header: []byte{1, 0, 0, 0, 0, 0, 14}, size: 26},
		{name: "checksum field past frame", header: []byte{1, 0, 0, 0, 0, 0, 12, 0, 1}, size: 26},
		{name: "large checksum offset", header: []byte{1, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}, size: 26},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := make([]byte, test.size)
			copy(packet, test.header)
			if _, err := virtioEthernetPacket(packet); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}
