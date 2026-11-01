package vz_test

import (
	"bytes"
	"net/netip"
	"testing"
)

const arpRequestHex = "ffffffffffff02000000000108060001080006040001020000000001c6336401000000000000c6336402"
const arpReplyHex = "02000000000102000000000b0806000108000604000202000000000bc6336402020000000001c6336401"

func TestARPReplyLiteral(t *testing.T) {
	for _, destination := range []struct {
		name string
		mac  []byte
	}{
		{"broadcast", []byte{255, 255, 255, 255, 255, 255}},
		{"unicast", packetPeerMAC[:]},
	} {
		t.Run(destination.name, func(t *testing.T) {
			request := packetBytes(t, arpRequestHex)
			copy(request[:6], destination.mac)
			request = append(request, bytes.Repeat([]byte{0xa5}, 18)...)
			original := bytes.Clone(request)
			want := packetBytes(t, arpReplyHex)
			got := arpReply(request, packetPeerIP, packetPeerMAC)
			if len(got) < len(want) || len(got) > 60 || !bytes.Equal(got[:len(want)], want) ||
				!bytes.Equal(got[len(want):], make([]byte, len(got)-len(want))) {
				t.Fatalf("ARP reply = %x, want %x followed by optional zero padding", got, want)
			}
			if !bytes.Equal(request, original) {
				t.Fatalf("input changed: got %x, want %x", request, original)
			}
		})
	}
}

func TestARPReplyRejectsUnrelatedOrInvalidFrames(t *testing.T) {
	request := packetBytes(t, arpRequestHex)
	mutated := func(edit func([]byte)) []byte {
		packet := bytes.Clone(request)
		edit(packet)
		return packet
	}
	tests := []struct {
		name  string
		frame []byte
		ip    netip.Addr
	}{
		{"empty", nil, packetPeerIP},
		{"short Ethernet", request[:13], packetPeerIP},
		{"short ARP", request[:41], packetPeerIP},
		{"IPv4 not ARP", mutated(func(p []byte) { p[13] = 0 }), packetPeerIP},
		{"other Ethernet destination", mutated(func(p []byte) { p[0] = 2 }), packetPeerIP},
		{"other hardware type", mutated(func(p []byte) { p[15] = 2 }), packetPeerIP},
		{"other protocol", mutated(func(p []byte) { p[16] = 0x86; p[17] = 0xdd }), packetPeerIP},
		{"bad hardware length", mutated(func(p []byte) { p[18] = 5 }), packetPeerIP},
		{"bad protocol length", mutated(func(p []byte) { p[19] = 16 }), packetPeerIP},
		{"reply not request", mutated(func(p []byte) { p[21] = 2 }), packetPeerIP},
		{"sender MAC mismatch", mutated(func(p []byte) { p[27] ^= 1 }), packetPeerIP},
		{"other target IP", request, netip.MustParseAddr("198.51.100.3")},
		{"IPv6 peer", request, netip.MustParseAddr("2001:db8::1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := bytes.Clone(tt.frame)
			if got := arpReply(tt.frame, tt.ip, packetPeerMAC); got != nil {
				t.Errorf("invalid frame returned %x, want nil", got)
			}
			if !bytes.Equal(tt.frame, original) {
				t.Errorf("input changed: got %x, want %x", tt.frame, original)
			}
		})
	}
}
