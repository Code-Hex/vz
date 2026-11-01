package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

const rscTestTCPHeaderSize = tcpHeaderSize + 12 // TCP header plus two NOPs and a timestamp option.

func rscTestPacket(v6 bool, kind byte, payload, size int) []byte {
	packet := gsoTestPacket(v6, true, payload, size)
	packet[0], packet[1] = virtioRSCInfo|virtioDataValid, kind
	if kind != 0 {
		packet[0] = virtioRSCInfo | virtioNeedsChecksum
	}
	// This coalescing count is deliberately unrelated to the payload. It must
	// not control segment boundaries or checksum field addresses.
	binary.LittleEndian.PutUint16(packet[virtioRSCSegmentsOffset:], 65535)
	clear(packet[virtioRSCDuplicateACKsOffset : virtioRSCDuplicateACKsOffset+2])
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
		{"TCPv4 GSO", false, virtioGSOTCPv4, 701, 250, 1460, []int{250, 250, 201}},
		{"TCPv6 ECN GSO", true, virtioGSOTCPv6 | virtioGSOECN, 701, 250, 1460, []int{250, 250, 201}},
		{"single segment checksum", false, 0, 37, 0, 1460, []int{37}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := gsoTestState(t, test.v6, test.mss)
			packet := rscTestPacket(test.v6, test.kind, test.payload, test.size)
			ipSize := ipv4HeaderSize
			if test.v6 {
				ipSize = ipv6HeaderSize
			}
			headers := ethernetHeaderSize + ipSize + rscTestTCPHeaderSize
			wantPayload := bytes.Clone(packet[virtioHeaderSize+headers:])
			var sizes []int
			var joined []byte
			err := state.normalizePeerPackets([][]byte{packet}, [][]byte{make([]byte, 1526)}, func(packets [][]byte) error {
				for _, packet := range packets {
					if !bytes.Equal(packet[:virtioHeaderSize], make([]byte, virtioHeaderSize)) {
						t.Fatal("RSC metadata reached the receiver")
					}
					frame := packet[virtioHeaderSize:]
					ip, tcp := frame[ethernetHeaderSize:], frame[ethernetHeaderSize+ipSize:]
					if len(ip) > 1500 || binary.BigEndian.Uint32(tcp[tcpSequenceOffset:]) != uint32(0xfffffff0)+uint32(len(joined)) {
						t.Fatal("invalid RSC output length or sequence")
					}
					flags := byte(0x50) // ACK and ECE on every segment.
					if len(joined) == 0 {
						flags |= 0x80 // CWR only on the first segment.
					}
					if len(joined)+len(tcp)-rscTestTCPHeaderSize == test.payload {
						flags |= 9 // FIN and PSH only on the last segment.
					}
					if tcp[tcpFlagsOffset] != flags {
						t.Fatalf("TCP flags = %#x, want %#x", tcp[tcpFlagsOffset], flags)
					}
					pseudo := bytes.Clone(ip[ipv4SourceOffset:ipv4HeaderSize])
					if test.v6 {
						pseudo = bytes.Clone(ip[ipv6SourceOffset:ipv6HeaderSize])
					} else if wireSum(ip[:ipv4HeaderSize]) != 65535 {
						t.Fatal("invalid IPv4 checksum")
					}
					pseudo = append(pseudo, 0, 6, byte(len(tcp)>>8), byte(len(tcp)))
					if wireSum(append(pseudo, tcp...)) != 65535 {
						t.Fatal("RSC checksum was not recomputed from the packet")
					}
					sizes = append(sizes, len(tcp)-rscTestTCPHeaderSize)
					joined = append(joined, tcp[rscTestTCPHeaderSize:]...)
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
		{"missing checksum validation", func(p []byte) { p[0] = virtioRSCInfo }},
		{"duplicate ACKs", func(p []byte) { p[virtioRSCDuplicateACKsOffset] = 1 }},
		{"wrong IP version", func(p []byte) { p[1] = virtioGSOTCPv6 }},
		{"zero GSO size", func(p []byte) { p[1] = virtioGSOTCPv4 }},
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
	packet[virtioHeaderSize+ethernetHeaderSize+ipv4HeaderSize+tcpFlagsOffset] = 0x10
	calls := 0
	if err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
		calls++
		if len(frames) != 1 || len(frames[0]) != 66 {
			t.Fatalf("ACK-only RSC output = %d frames", len(frames))
		}
		ip := frames[0][ethernetHeaderSize : ethernetHeaderSize+ipv4HeaderSize]
		tcp := frames[0][ethernetHeaderSize+ipv4HeaderSize:]
		pseudo := append(bytes.Clone(ip[ipv4SourceOffset:ipv4HeaderSize]), 0, 6, 0, 32)
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
	packet := make([]byte, virtioHeaderSize+ethernetHeaderSize)
	packet[0] = virtioRSCInfo | virtioDataValid
	binary.LittleEndian.PutUint16(packet[virtioRSCSegmentsOffset:], 65535)
	want := bytes.Clone(packet[virtioHeaderSize:])
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
