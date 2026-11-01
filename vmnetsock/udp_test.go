package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"testing"
)

const (
	udpTestIPStart          = virtioHeaderSize + ethernetHeaderSize
	udpTestIPv4UDPStart     = udpTestIPStart + ipv4HeaderSize
	udpTestIPv6UDPStart     = udpTestIPStart + ipv6HeaderSize
	udpTestIPv4PayloadStart = udpTestIPv4UDPStart + udpHeaderSize
)

func udpGSOPacket(v6 bool, payload, size int) []byte {
	ipSize := 20
	if v6 {
		ipSize = 40
	}
	p := make([]byte, udpTestIPStart+ipSize+udpHeaderSize+payload)
	p[0], p[1] = virtioNeedsChecksum, virtioGSOUDPL4
	binary.LittleEndian.PutUint16(p[2:], 65535) // hdr_len is only a hint.
	binary.LittleEndian.PutUint16(p[virtioGSOSizeOffset:], uint16(size))
	binary.LittleEndian.PutUint16(p[virtioChecksumStartOffset:], uint16(14+ipSize))
	binary.LittleEndian.PutUint16(p[virtioChecksumFieldOffset:], udpChecksumOffset)
	binary.LittleEndian.PutUint16(p[10:], 65535)
	frame := p[virtioHeaderSize:]
	copy(frame, []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1})
	ip := frame[ethernetHeaderSize:]
	if v6 {
		binary.BigEndian.PutUint16(frame[12:], 0x86dd)
		ip[0], ip[1], ip[2], ip[3] = 0x6a, 0xb1, 0x23, 0x45
		binary.BigEndian.PutUint16(ip[4:], uint16(8+payload))
		ip[6], ip[7] = 17, 63
		copy(ip[8:24], []byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
		copy(ip[24:40], []byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2})
	} else {
		binary.BigEndian.PutUint16(frame[12:], 0x0800)
		ip[0], ip[1], ip[8], ip[9] = 0x45, 0xab, 63, 17
		binary.BigEndian.PutUint16(ip[2:], uint16(28+payload))
		binary.BigEndian.PutUint16(ip[4:], 65535)
		binary.BigEndian.PutUint16(ip[6:], 0x4000)
		copy(ip[12:20], []byte{192, 0, 2, 1, 192, 0, 2, 2})
		binary.BigEndian.PutUint16(ip[10:], ^uint16(wireSum(ip[:20])))
	}
	udp := ip[ipSize:]
	binary.BigEndian.PutUint16(udp, 40000)
	binary.BigEndian.PutUint16(udp[2:], 9000)
	binary.BigEndian.PutUint16(udp[udpLengthOffset:], uint16(8+payload))
	binary.BigEndian.PutUint16(udp[udpChecksumOffset:], 0x1234)
	for i := range udp[udpHeaderSize:] {
		udp[udpHeaderSize+i] = byte(i*29 + 7)
	}
	return p
}

func udpWirePseudo(v6 bool, ip []byte, length int) []byte {
	if v6 {
		p := bytes.Clone(ip[8:40])
		return append(p, 0, 0, byte(length>>8), byte(length), 0, 0, 0, 17)
	}
	p := bytes.Clone(ip[12:20])
	return append(p, 0, 17, byte(length>>8), byte(length))
}

func checkUDPWire(t *testing.T, frame, input []byte, v6 bool, index, size int) {
	t.Helper()
	ipSize := 20
	if v6 {
		ipSize = 40
	}
	if len(frame) != 14+ipSize+8+size {
		t.Fatalf("datagram %d frame length = %d, want %d", index, len(frame), 14+ipSize+8+size)
	}
	if !bytes.Equal(frame[:14], input[virtioHeaderSize:udpTestIPStart]) {
		t.Fatal("Ethernet addresses or type changed")
	}
	ip, udp := frame[14:], frame[14+ipSize:]
	originalIP := input[udpTestIPStart:]
	if v6 {
		if binary.BigEndian.Uint16(ip[4:]) != uint16(8+size) || !bytes.Equal(ip[:4], originalIP[:4]) || !bytes.Equal(ip[6:40], originalIP[6:40]) {
			t.Fatal("IPv6 length, traffic class, flow label, addresses or hop limit changed")
		}
	} else {
		if binary.BigEndian.Uint16(ip[2:]) != uint16(28+size) || binary.BigEndian.Uint16(ip[4:]) != uint16(65535+index) || wireSum(ip[:20]) != 65535 {
			t.Fatal("invalid IPv4 length, ID or checksum")
		}
		if !bytes.Equal(ip[:2], originalIP[:2]) || !bytes.Equal(ip[6:10], originalIP[6:10]) || !bytes.Equal(ip[12:20], originalIP[12:20]) {
			t.Fatal("IPv4 flags, traffic class, protocol, addresses or TTL changed")
		}
	}
	if !bytes.Equal(udp[:4], originalIP[ipSize:ipSize+4]) || binary.BigEndian.Uint16(udp[udpLengthOffset:]) != uint16(8+size) {
		t.Fatal("UDP ports or length changed")
	}
	if binary.BigEndian.Uint16(udp[udpChecksumOffset:]) == 0 || wireSum(append(udpWirePseudo(v6, ip, len(udp)), udp...)) != 65535 {
		t.Fatal("invalid UDP checksum")
	}
}

func TestVirtioUDPGSO(t *testing.T) {
	for _, test := range []struct {
		name     string
		v6       bool
		payload  int
		size     int
		segments []int
	}{
		{"IPv4 exact boundary", false, 500, 250, []int{250, 250}},
		{"IPv6 exact boundary", true, 500, 250, []int{250, 250}},
		{"IPv4 odd and short last", false, 503, 251, []int{251, 251, 1}},
		{"IPv6 odd and short last", true, 503, 251, []int{251, 251, 1}},
		{"IPv4 single", false, 37, 250, []int{37}},
		{"IPv6 single", true, 37, 250, []int{37}},
		{"IPv4 MTU boundary", false, 2945, 1472, []int{1472, 1472, 1}},
		{"IPv6 MTU boundary", true, 2905, 1452, []int{1452, 1452, 1}},
	} {
		for _, peer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/peer=%v", test.name, peer), func(t *testing.T) {
				state := &adapterState{tcp: newTCPSegmentation(1500)}
				packet := udpGSOPacket(test.v6, test.payload, test.size)
				original := bytes.Clone(packet)
				ipSize := 20
				if test.v6 {
					ipSize = 40
				}
				var sizes []int
				var joined []byte
				write := func(frames [][]byte) error {
					for _, frame := range frames {
						if peer {
							if !bytes.Equal(frame[:12], make([]byte, 12)) {
								t.Fatal("normalized Virtio header is not zero")
							}
							frame = frame[12:]
						}
						if len(sizes) >= len(test.segments) {
							t.Fatal("extra UDP datagram")
						}
						checkUDPWire(t, frame, original, test.v6, len(sizes), test.segments[len(sizes)])
						sizes = append(sizes, len(frame)-14-ipSize-8)
						joined = append(joined, frame[14+ipSize+8:]...)
					}
					return nil
				}
				buffers := [][]byte{bytes.Repeat([]byte{0xa5}, 1526), bytes.Repeat([]byte{0xa5}, 1526)}
				var err error
				if peer {
					err = state.normalizePeerPackets([][]byte{packet}, buffers, write)
				} else {
					err = state.forwardVirtioPackets([][]byte{packet}, buffers, false, write)
				}
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(sizes, test.segments) || !bytes.Equal(joined, original[12+14+ipSize+8:]) {
					t.Fatalf("datagram lengths = %v, want %v; payload changed = %v", sizes, test.segments, !bytes.Equal(joined, original[12+14+ipSize+8:]))
				}
				if !bytes.Equal(packet, original) {
					t.Fatal("UDP GSO changed its borrowed input")
				}
			})
		}
	}
}

func TestVirtioUDPGSORejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name string
		v6   bool
		edit func([]byte) []byte
	}{
		{"missing checksum flag", false, func(p []byte) []byte { p[0] = 0; return p }},
		{"RSC", false, func(p []byte) []byte { p[0] |= 4; return p }},
		{"ECN modifier", false, func(p []byte) []byte { p[1] = 0x85; return p }},
		{"UFO is not UDP L4", false, func(p []byte) []byte { p[1] = 3; return p }},
		{"wrong checksum start", false, func(p []byte) []byte { p[virtioChecksumStartOffset]++; return p }},
		{"wrong checksum offset", false, func(p []byte) []byte { p[virtioChecksumFieldOffset]++; return p }},
		{"zero segment size", false, func(p []byte) []byte { binary.LittleEndian.PutUint16(p[virtioGSOSizeOffset:], 0); return p }},
		{"IPv4 size exceeds MTU", false, func(p []byte) []byte { binary.LittleEndian.PutUint16(p[virtioGSOSizeOffset:], 1473); return p }},
		{"IPv6 size exceeds MTU", true, func(p []byte) []byte { binary.LittleEndian.PutUint16(p[virtioGSOSizeOffset:], 1453); return p }},
		{"IPv4 UDP length too short", false, func(p []byte) []byte {
			binary.BigEndian.PutUint16(p[udpTestIPv4UDPStart+udpLengthOffset:], 258)
			return p
		}},
		{"IPv6 UDP length too long", true, func(p []byte) []byte {
			binary.BigEndian.PutUint16(p[udpTestIPv6UDPStart+udpLengthOffset:], 260)
			return p
		}},
		{"IPv4 empty payload", false, func([]byte) []byte { return udpGSOPacket(false, 0, 100) }},
		{"IPv6 empty payload", true, func([]byte) []byte { return udpGSOPacket(true, 0, 100) }},
		{"IPv4 length exceeds frame", false, func(p []byte) []byte { p[udpTestIPStart+ipv4TotalLengthOffset+1]++; return p }},
		{"IPv6 length exceeds frame", true, func(p []byte) []byte { p[udpTestIPStart+ipv6PayloadLengthOffset+1]++; return p }},
		{"IPv4 trailing bytes", false, func(p []byte) []byte { return append(p, 0) }},
		{"IPv6 trailing bytes", true, func(p []byte) []byte { return append(p, 0) }},
		{"IPv4 zero length", false, func(p []byte) []byte {
			binary.BigEndian.PutUint16(p[udpTestIPStart+ipv4TotalLengthOffset:], 0)
			return p
		}},
		{"IPv6 zero length", true, func(p []byte) []byte {
			binary.BigEndian.PutUint16(p[udpTestIPStart+ipv6PayloadLengthOffset:], 0)
			return p
		}},
		{"IPv4 options", false, func(p []byte) []byte { p[udpTestIPStart] = 0x46; return p }},
		{"IPv4 more fragments", false, func(p []byte) []byte { p[udpTestIPStart+ipv4FragmentFlagsOffset] |= 0x20; return p }},
		{"IPv4 fragment offset", false, func(p []byte) []byte { p[udpTestIPStart+ipv4FragmentFlagsOffset+1] = 1; return p }},
		{"IPv6 hop by hop", true, func(p []byte) []byte { p[udpTestIPStart+ipv6NextHeaderOffset] = 0; return p }},
		{"IPv6 routing", true, func(p []byte) []byte { p[udpTestIPStart+ipv6NextHeaderOffset] = 43; return p }},
		{"IPv6 fragment", true, func(p []byte) []byte { p[udpTestIPStart+ipv6NextHeaderOffset] = 44; return p }},
		{"IPv6 authentication", true, func(p []byte) []byte { p[udpTestIPStart+ipv6NextHeaderOffset] = 51; return p }},
		{"IPv6 destination options", true, func(p []byte) []byte { p[udpTestIPStart+ipv6NextHeaderOffset] = 60; return p }},
		{"VLAN", false, func(p []byte) []byte {
			binary.BigEndian.PutUint16(p[virtioHeaderSize+etherTypeOffset:], 0x8100)
			return p
		}},
		{"QinQ", false, func(p []byte) []byte {
			binary.BigEndian.PutUint16(p[virtioHeaderSize+etherTypeOffset:], 0x88a8)
			return p
		}},
		{"TCP payload", false, func(p []byte) []byte { p[udpTestIPStart+ipv4ProtocolOffset] = 6; return p }},
		{"short Virtio header", false, func(p []byte) []byte { return p[:11] }},
		{"short UDP header", false, func(p []byte) []byte { return p[:udpTestIPv4UDPStart+udpHeaderSize-1] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &adapterState{tcp: newTCPSegmentation(1500)}
			packet := test.edit(udpGSOPacket(test.v6, 251, 100))
			written := 0
			err := state.forwardVirtioPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
				written += len(frames)
				return nil
			})
			if err == nil || written != 0 {
				t.Fatalf("invalid input: error = %v, written = %d", err, written)
			}
		})
	}
	state := &adapterState{}
	if err := state.normalizePeerPackets([][]byte{udpGSOPacket(false, 251, 100)}, nil, func([][]byte) error {
		t.Fatal("UDP GSO without TSO emitted packets")
		return nil
	}); err == nil {
		t.Fatal("UDP GSO without TSO was accepted")
	}
}

func TestVirtioUDPGSOZeroChecksum(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		t.Run(fmt.Sprintf("IPv6=%v", v6), func(t *testing.T) {
			p := udpGSOPacket(v6, 2, 2)
			ipSize := 20
			if v6 {
				ipSize = 40
			}
			ip, udp := p[udpTestIPStart:], p[udpTestIPStart+ipSize:]
			clear(udp[udpChecksumOffset:]) // A partial checksum seed of zero is valid.
			binary.BigEndian.PutUint16(udp[udpHeaderSize:], ^uint16(wireSum(append(udpWirePseudo(v6, ip, 10), udp...))))
			if wireSum(append(udpWirePseudo(v6, ip, 10), udp...)) != 65535 {
				t.Fatal("test packet does not require a zero checksum")
			}
			state := &adapterState{tcp: newTCPSegmentation(1500)}
			written := 0
			err := state.forwardVirtioPackets([][]byte{p}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
				for _, frame := range frames {
					written++
					checkUDPWire(t, frame, p, v6, 0, 2)
					if binary.BigEndian.Uint16(frame[ethernetHeaderSize+ipSize+udpChecksumOffset:]) != 65535 {
						t.Fatal("UDP checksum zero was not encoded as 0xffff")
					}
				}
				return nil
			})
			if err != nil || written != 1 {
				t.Fatalf("zero checksum: error = %v, written = %d", err, written)
			}
		})
	}
}

func TestVirtioUDPGSOPadding(t *testing.T) {
	p := udpGSOPacket(false, 1, 1)
	p = append(p, bytes.Repeat([]byte{0xa5}, 17)...)
	state := &adapterState{tcp: newTCPSegmentation(1500)}
	written := 0
	err := state.forwardVirtioPackets([][]byte{p}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
		for _, frame := range frames {
			written++
			checkUDPWire(t, frame, p, false, 0, 1)
			if frame[42] != 7 {
				t.Fatal("Ethernet padding entered the UDP payload")
			}
		}
		return nil
	})
	if err != nil || written != 1 {
		t.Fatalf("padded UDP GSO: error = %v, written = %d", err, written)
	}
	if err := state.forwardVirtioPackets([][]byte{append(p, 0)}, [][]byte{make([]byte, 1514)}, false, func([][]byte) error {
		t.Fatal("excess padding emitted a datagram")
		return nil
	}); err == nil {
		t.Fatal("padding past the Ethernet minimum was accepted")
	}
}

func TestVirtioUDPGSOBatchesAndWriteError(t *testing.T) {
	packet := udpGSOPacket(false, 2177, 64)
	want := bytes.Clone(packet[udpTestIPv4PayloadStart:])
	state := &adapterState{tcp: newTCPSegmentation(1500)}
	buffers := make([][]byte, 32)
	for i := range buffers {
		buffers[i] = bytes.Repeat([]byte{0xa5}, 1526)
	}
	var batches []int
	var joined []byte
	count := 0
	err := state.normalizePeerPackets([][]byte{packet}, buffers, func(frames [][]byte) error {
		batches = append(batches, len(frames))
		for _, frame := range frames {
			if !bytes.Equal(frame[:12], make([]byte, 12)) {
				t.Fatal("stale output Virtio metadata")
			}
			size := 64
			if count == 34 {
				size = 1
			}
			checkUDPWire(t, frame[12:], packet, false, count, size)
			joined = append(joined, frame[udpTestIPv4PayloadStart:]...)
			count++
		}
		return nil
	})
	if err != nil || !slices.Equal(batches, []int{32, 3}) || !bytes.Equal(joined, want) {
		t.Fatalf("UDP batches: error = %v, sizes = %v, payload equal = %v", err, batches, bytes.Equal(joined, want))
	}
	wantErr := errors.New("peer closed")
	calls := 0
	err = state.normalizePeerPackets([][]byte{packet}, buffers, func([][]byte) error {
		calls++
		return wantErr
	})
	if !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("UDP write error = %v, calls = %d", err, calls)
	}
}
