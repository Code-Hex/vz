package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"
)

func gsoTestState(t *testing.T, v6 bool, mss uint16) *adapterState {
	t.Helper()
	peer, remote := netip.MustParseAddrPort("192.0.2.2:1000"), netip.MustParseAddrPort("192.0.2.1:80")
	if v6 {
		peer, remote = netip.MustParseAddrPort("[2001:db8::2]:1000"), netip.MustParseAddrPort("[2001:db8::1]:80")
	}
	state := &adapterState{
		tcp: newTCPSegmentation(1500),
	}
	now := time.Now()
	if err := state.tcp.observePeer(flowPacket(peer, remote, 2, 10, 0, mss, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := state.tcp.forwardNative(flowPacket(remote, peer, 0x12, 20, 11, 1460, 0), now, func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return state
}

func gsoTestPacket(v6, ecn bool, payload, size int) []byte {
	src, dst := netip.MustParseAddrPort("192.0.2.1:80"), netip.MustParseAddrPort("192.0.2.2:1000")
	ipSize, kind := 20, byte(1)
	if v6 {
		src, dst = netip.MustParseAddrPort("[2001:db8::1]:80"), netip.MustParseAddrPort("[2001:db8::2]:1000")
		ipSize, kind = 40, 4
	}
	flags := byte(0x19)
	if ecn {
		kind |= 0x80
		flags |= 0xc0
	}
	frame := flowPacket(src, dst, flags, 0xfffffff0, 11, 0, payload)
	tcpStart := 14 + ipSize
	frame = slices.Insert(frame, tcpStart+20, []byte{1, 1, 8, 10, 0, 0, 0, 1, 0, 0, 0, 2}...)
	frame[tcpStart+12] = 8 << 4
	binary.BigEndian.PutUint16(frame[tcpStart+16:], 0x1234)
	if v6 {
		binary.BigEndian.PutUint16(frame[18:], uint16(32+payload))
	} else {
		binary.BigEndian.PutUint16(frame[16:], uint16(20+32+payload))
		binary.BigEndian.PutUint16(frame[18:], 65535)
	}
	packet := append(make([]byte, 12), frame...)
	packet[0], packet[1] = 1, kind
	binary.LittleEndian.PutUint16(packet[4:], uint16(size))
	binary.LittleEndian.PutUint16(packet[6:], uint16(tcpStart))
	binary.LittleEndian.PutUint16(packet[8:], 16)
	binary.LittleEndian.PutUint16(packet[10:], 65535)
	return packet
}

func TestVirtioTCPGSO(t *testing.T) {
	for _, test := range []struct {
		name     string
		v6, ecn  bool
		payload  int
		size     int
		mss      uint16
		segments []int
	}{
		{"IPv4 ECN", false, true, 701, 250, 1460, []int{250, 250, 201}},
		{"IPv6 ECN", true, true, 701, 250, 1460, []int{250, 250, 201}},
		{"single segment", false, false, 37, 250, 1460, []int{37}},
		{"peer MSS", true, false, 701, 250, 200, []int{188, 188, 188, 137}},
		{"MTU", false, false, 3001, 2000, 9000, []int{1448, 1448, 105}},
		{"many batches", true, false, 2101, 60, 1460, append(slices.Repeat([]int{60}, 35), 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := gsoTestState(t, test.v6, test.mss)
			packet := gsoTestPacket(test.v6, test.ecn, test.payload, test.size)
			if test.v6 {
				packet[0] |= 0x80
				binary.LittleEndian.PutUint16(packet[2:], 65535)
			}
			ipSize := 20
			if test.v6 {
				ipSize = 40
			}
			headers := 14 + ipSize + 32
			wantPayload := bytes.Clone(packet[12+headers:])
			buffers := [][]byte{make([]byte, 1514), make([]byte, 1514)}
			var sizes []int
			var joined []byte
			batches := 0
			err := state.forwardTCPPackets([][]byte{packet}, buffers, false, func(frames [][]byte) error {
				batches++
				if len(frames) > 2 {
					t.Fatal("output exceeded the available batch buffers")
				}
				for _, frame := range frames {
					if len(frame) < headers || len(frame) > 1514 {
						t.Fatalf("invalid output size %d", len(frame))
					}
					ip, tcp := frame[14:], frame[14+ipSize:]
					if binary.BigEndian.Uint32(tcp[4:]) != uint32(0xfffffff0)+uint32(len(joined)) {
						t.Fatal("segment sequence changed")
					}
					last := len(joined)+len(frame)-headers == test.payload
					flags := byte(0x10)
					if test.ecn {
						flags |= 0x40
						if len(sizes) == 0 {
							flags |= 0x80
						}
					}
					if last {
						flags |= 9
					}
					if tcp[13] != flags {
						t.Fatalf("TCP flags = %#x, want %#x", tcp[13], flags)
					}
					var pseudo []byte
					if test.v6 {
						if int(binary.BigEndian.Uint16(ip[4:])) != len(tcp) {
							t.Fatal("IPv6 payload length changed")
						}
						pseudo = append(pseudo, ip[8:40]...)
						pseudo = append(pseudo, 0, 0, byte(len(tcp)>>8), byte(len(tcp)), 0, 0, 0, 6)
					} else {
						if int(binary.BigEndian.Uint16(ip[2:])) != len(ip) || binary.BigEndian.Uint16(ip[4:]) != uint16(65535+len(sizes)) || wireSum(ip[:20]) != 65535 {
							t.Fatal("IPv4 length, ID or checksum changed")
						}
						pseudo = append(pseudo, ip[12:20]...)
						pseudo = append(pseudo, 0, 6, byte(len(tcp)>>8), byte(len(tcp)))
					}
					if wireSum(append(pseudo, tcp...)) != 65535 {
						t.Fatal("invalid TCP checksum")
					}
					sizes = append(sizes, len(frame)-headers)
					joined = append(joined, frame[headers:]...)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(sizes, test.segments) || !bytes.Equal(joined, wantPayload) {
				t.Fatalf("segments = %v, want %v; payload equal = %t", sizes, test.segments, bytes.Equal(joined, wantPayload))
			}
			if batches != (len(test.segments)+1)/2 {
				t.Fatalf("batches = %d", batches)
			}
		})
	}
}

func TestVirtioTCPGSOInvalidMetadata(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func([]byte)
	}{
		{"zero segment size", func(p []byte) { clear(p[4:6]) }},
		{"missing checksum flag", func(p []byte) { p[0] = 0 }},
		{"RSC duplicate ACKs", func(p []byte) { p[0] = virtioRSCInfo | virtioNeedsChecksum }},
		{"wrong IP version", func(p []byte) { p[1] = 4 }},
		{"wrong checksum start", func(p []byte) { p[6]++ }},
		{"wrong checksum offset", func(p []byte) { p[8]++ }},
		{"UFO", func(p []byte) { p[1] = 3 }},
		{"UDP L4", func(p []byte) { p[1] = 5 }},
		{"ECN alone", func(p []byte) { p[1] = 0x80 }},
		{"unknown GSO", func(p []byte) { p[1] = 0x41 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := gsoTestState(t, false, 1460)
			packet := gsoTestPacket(false, false, 701, 250)
			test.edit(packet)
			written := 0
			err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
				written += len(frames)
				return nil
			})
			if err == nil || written != 0 {
				t.Fatalf("invalid metadata: written=%d, error=%v", written, err)
			}
		})
	}
	state := gsoTestState(t, false, 1460)
	valid := gsoTestPacket(false, false, 701, 250)
	for _, packet := range [][]byte{nil, valid[:1], valid[:25], valid[:26], gsoTestPacket(false, false, 0, 250)} {
		if err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func([][]byte) error {
			t.Fatal("short or empty GSO emitted data")
			return nil
		}); err == nil {
			t.Fatal("short or empty GSO accepted")
		}
	}
	state = &adapterState{
		tcp: newTCPSegmentation(1500),
	}
	if err := state.forwardTCPPackets([][]byte{gsoTestPacket(false, false, 701, 250)}, [][]byte{make([]byte, 1514)}, false, func([][]byte) error {
		t.Fatal("unobserved connection emitted data")
		return nil
	}); err == nil {
		t.Fatal("unobserved connection accepted GSO data")
	}
}

func TestVirtioTCPGSOWriteError(t *testing.T) {
	state := gsoTestState(t, false, 1460)
	want := errors.New("peer closed")
	calls := 0
	err := state.forwardTCPPackets([][]byte{gsoTestPacket(false, false, 2101, 60)}, [][]byte{make([]byte, 1514), make([]byte, 1514)}, false, func([][]byte) error {
		calls++
		return want
	})
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("write failure = %v, calls=%d", err, calls)
	}
}

func TestVirtioTCPGSOFrameLength(t *testing.T) {
	for _, test := range []struct {
		name string
		v6   bool
	}{
		{"IPv4", false},
		{"IPv6", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := gsoTestState(t, test.v6, 1460)
			packet := gsoTestPacket(test.v6, false, 701, 250)
			lengthOffset := 12 + 16
			if test.v6 {
				lengthOffset = 12 + 18
			}
			length := binary.BigEndian.Uint16(packet[lengthOffset:])
			binary.BigEndian.PutUint16(packet[lengthOffset:], length-64)
			written := 0
			err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
				written += len(frames)
				return nil
			})
			if err == nil || written != 0 {
				t.Fatalf("truncated GSO payload: written=%d, error=%v", written, err)
			}
		})
	}

	state := gsoTestState(t, false, 1460)
	src, dst := netip.MustParseAddrPort("192.0.2.1:80"), netip.MustParseAddrPort("192.0.2.2:1000")
	frame := flowPacket(src, dst, 0x18, 21, 11, 0, 1)
	frame = append(frame, bytes.Repeat([]byte{0xa5}, 5)...)
	packet := append(make([]byte, 12), frame...)
	packet[0], packet[1] = 1, 1
	binary.LittleEndian.PutUint16(packet[4:], 250)
	binary.LittleEndian.PutUint16(packet[6:], 34)
	binary.LittleEndian.PutUint16(packet[8:], 16)
	written := 0
	err := state.forwardTCPPackets([][]byte{packet}, [][]byte{make([]byte, 1514)}, false, func(frames [][]byte) error {
		for _, frame := range frames {
			written++
			if len(frame) != 55 || frame[54] != 20 || wireSum(frame[14:34]) != 65535 {
				t.Fatal("Ethernet padding changed the TCP payload or IP checksum")
			}
			pseudo := append([]byte(nil), frame[26:34]...)
			pseudo = append(pseudo, 0, 6, 0, 21)
			if wireSum(append(pseudo, frame[34:]...)) != 65535 {
				t.Fatal("invalid padded TCP checksum")
			}
		}
		return nil
	})
	if err != nil || written != 1 {
		t.Fatalf("padded GSO: written=%d, error=%v", written, err)
	}
}
