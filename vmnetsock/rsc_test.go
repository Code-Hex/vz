package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

func rscTestPacket(v6 bool, kind byte, payload, size int) []byte {
	packet := gsoTestPacket(v6, true, payload, size)
	packet[0], packet[1] = 6, kind
	if kind != 0 {
		packet[0] = 5
	}
	binary.LittleEndian.PutUint16(packet[6:], 65535)
	clear(packet[8:10])
	return packet
}

func TestVirtioTCPRSC(t *testing.T) {
	for _, test := range []struct {
		name     string
		v6       bool
		kind     byte
		payload  int
		size     int
		mss      uint16
		segments []int
	}{
		{"IPv4 without GSO size", false, 0, 3001, 0, 1460, []int{1448, 1448, 105}},
		{"IPv6 receiver MSS", true, 0, 701, 0, 200, []int{188, 188, 188, 137}},
		{"TCPv4 GSO", false, 1, 701, 250, 1460, []int{250, 250, 201}},
		{"TCPv6 ECN GSO", true, 0x84, 701, 250, 1460, []int{250, 250, 201}},
		{"single segment checksum", false, 0, 37, 0, 1460, []int{37}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := gsoTestState(t, test.v6, test.mss)
			packet := rscTestPacket(test.v6, test.kind, test.payload, test.size)
			ipSize := 20
			if test.v6 {
				ipSize = 40
			}
			headers := 14 + ipSize + 32
			wantPayload := bytes.Clone(packet[12+headers:])
			var sizes []int
			var joined []byte
			err := state.normalizePeerPackets([][]byte{packet}, [][]byte{make([]byte, 1526)}, func(packets [][]byte) error {
				for _, packet := range packets {
					if !bytes.Equal(packet[:12], make([]byte, 12)) {
						t.Fatal("RSC metadata reached the receiver")
					}
					frame := packet[12:]
					ip, tcp := frame[14:], frame[14+ipSize:]
					if len(ip) > 1500 || binary.BigEndian.Uint32(tcp[4:]) != uint32(0xfffffff0)+uint32(len(joined)) {
						t.Fatal("invalid RSC output length or sequence")
					}
					flags := byte(0x50)
					if len(joined) == 0 {
						flags |= 0x80
					}
					if len(joined)+len(tcp)-32 == test.payload {
						flags |= 9
					}
					if tcp[13] != flags {
						t.Fatalf("TCP flags = %#x, want %#x", tcp[13], flags)
					}
					pseudo := bytes.Clone(ip[12:20])
					if test.v6 {
						pseudo = bytes.Clone(ip[8:40])
					} else if wireSum(ip[:20]) != 65535 {
						t.Fatal("invalid IPv4 checksum")
					}
					pseudo = append(pseudo, 0, 6, byte(len(tcp)>>8), byte(len(tcp)))
					if wireSum(append(pseudo, tcp...)) != 65535 {
						t.Fatal("RSC checksum was not recomputed from the packet")
					}
					sizes = append(sizes, len(tcp)-32)
					joined = append(joined, tcp[32:]...)
				}
				return nil
			})
			if err != nil || !slices.Equal(sizes, test.segments) || !bytes.Equal(joined, wantPayload) {
				t.Fatalf("RSC conversion = %v, segments = %v, payload equal = %v", err, sizes, bytes.Equal(joined, wantPayload))
			}
		})
	}
}

func TestVirtioTCPRSCRejectsUnsupportedMetadata(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func([]byte)
	}{
		{"missing checksum validation", func(p []byte) { p[0] = 4 }},
		{"duplicate ACKs", func(p []byte) { p[8] = 1 }},
		{"wrong IP version", func(p []byte) { p[1] = 4 }},
		{"zero GSO size", func(p []byte) { p[1] = 1 }},
		{"UFO", func(p []byte) { p[1] = 3 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := gsoTestState(t, false, 1460)
			packet := rscTestPacket(false, 0, 37, 0)
			test.edit(packet)
			err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func([][]byte) error {
				t.Fatal("unsupported RSC metadata emitted a packet")
				return nil
			})
			if err == nil {
				t.Fatal("unsupported RSC metadata accepted")
			}
		})
	}
}

func TestVirtioTCPRSCAcknowledgmentAndErrors(t *testing.T) {
	state := gsoTestState(t, false, 1460)
	packet := rscTestPacket(false, 0, 0, 0)
	packet[12+14+20+13] = 0x10
	calls := 0
	if err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
		calls++
		if len(frames) != 1 || len(frames[0]) != 66 {
			t.Fatalf("ACK-only RSC output = %d frames", len(frames))
		}
		ip, tcp := frames[0][14:34], frames[0][34:]
		pseudo := append(bytes.Clone(ip[12:20]), 0, 6, 0, 32)
		if wireSum(ip) != 65535 || wireSum(append(pseudo, tcp...)) != 65535 {
			t.Fatal("ACK-only RSC checksum is incomplete")
		}
		return nil
	}); err != nil || calls != 1 {
		t.Fatalf("ACK-only RSC = %v, writes = %d", err, calls)
	}
	packet = rscTestPacket(false, 0, 3001, 0)
	state = &adapterState{tcp: newTCPSegmentation(1500)}
	if err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func([][]byte) error {
		t.Fatal("unknown connection emitted RSC data")
		return nil
	}); err == nil {
		t.Fatal("RSC data accepted without an observed handshake")
	}
	state = gsoTestState(t, false, 1460)
	want := errors.New("receiver closed")
	calls = 0
	err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func([][]byte) error {
		calls++
		return want
	})
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("RSC write error = %v, writes = %d", err, calls)
	}
}

func TestVirtioRSCNonTCPIsUnchanged(t *testing.T) {
	state := gsoTestState(t, false, 1460)
	packet := make([]byte, 12+14)
	packet[0] = 6
	binary.LittleEndian.PutUint16(packet[6:], 65535)
	want := bytes.Clone(packet[12:])
	calls := 0
	if err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
		calls++
		if len(frames) != 1 || !bytes.Equal(frames[0], want) {
			t.Fatal("non-TCP RSC Ethernet bytes changed")
		}
		return nil
	}); err != nil || calls != 1 {
		t.Fatalf("non-TCP RSC = %v, writes = %d", err, calls)
	}
}
