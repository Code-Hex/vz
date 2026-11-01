package vz_test

import (
	"bytes"
	"encoding/hex"
	"net/netip"
	"testing"
)

var packetPeerIP = netip.MustParseAddr("198.51.100.2")
var packetPeerMAC = [6]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x0b}

const echoRequestHex = "02000000000b020000000001080045000021456740004001a10ac6336401c63364020800e5f81234000270696e6721"
const echoReplyHex = "02000000000102000000000b080045000021456740004001a10ac6336402c63364010000edf81234000270696e6721"

func packetBytes(t *testing.T, literal string) []byte {
	t.Helper()
	packet, err := hex.DecodeString(literal)
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func TestEchoReplyLiteral(t *testing.T) {
	request := packetBytes(t, echoRequestHex)
	original := bytes.Clone(request)
	want := packetBytes(t, echoReplyHex)
	got := echoReply(request, packetPeerIP, packetPeerMAC)
	if !bytes.Equal(got, want) {
		t.Fatalf("reply = %x, want %x", got, want)
	}
	if !bytes.Equal(request, original) {
		t.Fatalf("input changed: got %x, want %x", request, original)
	}
}

func TestEchoReplyIgnoresEthernetPadding(t *testing.T) {
	request := packetBytes(t, echoRequestHex)
	request = append(request, bytes.Repeat([]byte{0xa5}, 13)...)
	original := bytes.Clone(request)
	want := packetBytes(t, echoReplyHex)
	got := echoReply(request, packetPeerIP, packetPeerMAC)
	if len(got) < len(want) || len(got) > len(request) || !bytes.Equal(got[:len(want)], want) {
		t.Fatalf("reply with padding = %x, want exact IP packet %x and at most Ethernet padding", got, want)
	}
	if !bytes.Equal(request, original) {
		t.Fatalf("input with padding changed: got %x, want %x", request, original)
	}
}

func TestEchoReplyRejectsUnrelatedOrInvalidFrames(t *testing.T) {
	request := packetBytes(t, echoRequestHex)
	mutated := func(edit func([]byte)) []byte {
		packet := bytes.Clone(request)
		edit(packet)
		return packet
	}
	tests := []struct {
		name  string
		frame []byte
		ip    netip.Addr
		mac   [6]byte
	}{
		{"empty", nil, packetPeerIP, packetPeerMAC},
		{"short Ethernet header", request[:13], packetPeerIP, packetPeerMAC},
		{"short IPv4 header", request[:33], packetPeerIP, packetPeerMAC},
		{"short ICMP header", request[:41], packetPeerIP, packetPeerMAC},
		{"truncated payload", request[:len(request)-1], packetPeerIP, packetPeerMAC},
		{"ARP rather than IPv4", mutated(func(p []byte) { p[12], p[13] = 0x08, 0x06 }), packetPeerIP, packetPeerMAC},
		{"IPv6 rather than IPv4", mutated(func(p []byte) { p[12], p[13] = 0x86, 0xdd }), packetPeerIP, packetPeerMAC},
		{"wrong destination MAC", mutated(func(p []byte) { p[5] ^= 1 }), packetPeerIP, packetPeerMAC},
		{"wrong destination IP", request, netip.MustParseAddr("198.51.100.3"), packetPeerMAC},
		{"non IPv4 peer", request, netip.MustParseAddr("2001:db8::1"), packetPeerMAC},
		{"invalid IPv4 version", mutated(func(p []byte) { p[14] = 0x65 }), packetPeerIP, packetPeerMAC},
		{"invalid IPv4 header length", mutated(func(p []byte) { p[14] = 0x44 }), packetPeerIP, packetPeerMAC},
		{"IPv4 options exceed packet", mutated(func(p []byte) { p[14] = 0x4f }), packetPeerIP, packetPeerMAC},
		{"IP length less than header", mutated(func(p []byte) { p[16], p[17] = 0, 19 }), packetPeerIP, packetPeerMAC},
		{"IP length exceeds frame", mutated(func(p []byte) { p[16], p[17] = 0, 60 }), packetPeerIP, packetPeerMAC},
		{"corrupt IPv4 checksum", mutated(func(p []byte) { p[24] ^= 1 }), packetPeerIP, packetPeerMAC},
		{"corrupt ICMP checksum", mutated(func(p []byte) { p[36] ^= 1 }), packetPeerIP, packetPeerMAC},
		{"corrupt odd payload", mutated(func(p []byte) { p[len(p)-1] ^= 1 }), packetPeerIP, packetPeerMAC},
		{"IPv4 fragment more follows", mutated(func(p []byte) { p[20], p[21], p[24], p[25] = 0x20, 0, 0xc1, 0x0a }), packetPeerIP, packetPeerMAC},
		{"IPv4 fragment offset", mutated(func(p []byte) { p[20], p[21], p[24], p[25] = 0x40, 1, 0xa1, 0x09 }), packetPeerIP, packetPeerMAC},
		{"UDP rather than ICMP", mutated(func(p []byte) { p[23], p[24], p[25] = 17, 0xa0, 0xfa }), packetPeerIP, packetPeerMAC},
		{"ICMP reply rather than request", mutated(func(p []byte) { p[34], p[36], p[37] = 0, 0xed, 0xf8 }), packetPeerIP, packetPeerMAC},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := bytes.Clone(tt.frame)
			if got := echoReply(tt.frame, tt.ip, tt.mac); got != nil {
				t.Errorf("invalid frame returned reply %x, want nil", got)
			}
			if !bytes.Equal(tt.frame, original) {
				t.Errorf("input changed: got %x, want %x", tt.frame, original)
			}
		})
	}
}
