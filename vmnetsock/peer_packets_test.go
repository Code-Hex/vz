package vmnetsock

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"testing"
)

func TestNormalizePeerPacketsWithoutTSO(t *testing.T) {
	state := &adapterState{}
	packet := make([]byte, 12+19)
	packet[0] = 1
	binary.LittleEndian.PutUint16(packet[6:], 14)
	copy(packet[26:], []byte{0x12, 0x34, 0x56, 0x78, 0x9a})
	calls := 0
	if err := state.normalizePeerPackets([][]byte{packet}, nil, func(packets [][]byte) error {
		calls++
		if len(packets) != 1 || !bytes.Equal(packets[0][:12], make([]byte, 12)) || !bytes.Equal(packets[0][26:], []byte{0xfd, 0x52, 0x56, 0x78, 0x9a}) {
			t.Fatalf("normalized packet = %x", packets)
		}
		return nil
	}); err != nil || calls != 1 {
		t.Fatalf("normalization = %v, writes = %d", err, calls)
	}
	calls = 0
	if err := state.normalizePeerPackets([][]byte{gsoTestPacket(false, false, 701, 250)}, nil, func([][]byte) error {
		calls++
		return nil
	}); err == nil || calls != 0 {
		t.Fatalf("GSO without TSO = %v, writes = %d", err, calls)
	}
}

func TestNormalizePeerPacketsRejectsRSCBeforeWrite(t *testing.T) {
	for _, tso := range []bool{false, true} {
		for _, flags := range []byte{4, 6} {
			t.Run(fmt.Sprintf("TSO=%v/flags=%d", tso, flags), func(t *testing.T) {
				state := &adapterState{}
				if tso {
					state = gsoTestState(t, false, 1460)
				}
				packet := make([]byte, 12+14)
				packet[0] = flags
				calls := 0
				err := state.normalizePeerPackets([][]byte{packet}, [][]byte{make([]byte, 1526)}, func([][]byte) error {
					calls++
					return nil
				})
				if err == nil || calls != 0 {
					t.Fatalf("RSC normalization = %v, writes = %d", err, calls)
				}
			})
		}
	}
}

func TestPeerVirtioTCPHandshakeAndGSO(t *testing.T) {
	for _, test := range []struct {
		name           string
		v6             bool
		ifaceInitiated bool
	}{
		{name: "IPv4 iface SYN", ifaceInitiated: true},
		{name: "IPv6 socket SYN", v6: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			iface, socket := netip.MustParseAddrPort("192.0.2.2:1000"), netip.MustParseAddrPort("192.0.2.1:80")
			if test.v6 {
				iface, socket = netip.MustParseAddrPort("[2001:db8::2]:1000"), netip.MustParseAddrPort("[2001:db8::1]:80")
			}
			state := &adapterState{tcp: newTCPSegmentation(1500)}
			buffers := [][]byte{make([]byte, 1526), make([]byte, 1526)}
			flags, ack := byte(2), uint32(0)
			if !test.ifaceInitiated {
				packet := append(make([]byte, 12), flowPacket(socket, iface, 2, 20, 0, 1460, 0)...)
				if err := state.normalizePeerPackets([][]byte{packet}, buffers, func([][]byte) error { return nil }); err != nil {
					t.Fatal(err)
				}
				flags, ack = 0x12, 21
			}
			native := append(make([]byte, 12), flowPacket(iface, socket, flags, 10, ack, 200, 0)...)
			want := bytes.Clone(native)
			if err := state.forwardObservedPackets([][]byte{native}, func(packets [][]byte) error {
				if len(packets) != 1 || !bytes.Equal(packets[0], want) {
					t.Fatalf("native Virtio bytes changed: %x", packets)
				}
				if test.ifaceInitiated {
					response := append(make([]byte, 12), flowPacket(socket, iface, 0x12, 20, 11, 1460, 0)...)
					if err := state.normalizePeerPackets([][]byte{response}, buffers, func([][]byte) error { return nil }); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			data := flowPacket(iface, socket, 0x18, 11, 21, 0, 701)
			parsed, err := parseTCPPacket(data)
			if err != nil {
				t.Fatal(err)
			}
			parsed.updateSegmentHeaders(data[14:], 0)
			native = append(make([]byte, 12), data...)
			native[0] = 4
			if test.v6 {
				native[0] = 6
			}
			binary.LittleEndian.PutUint16(native[6:], 3)
			want = bytes.Clone(native)
			if err := state.forwardObservedPackets([][]byte{native}, func(packets [][]byte) error {
				if len(packets) != 1 || !bytes.Equal(packets[0], want) {
					t.Fatalf("established native RSC packet changed: %x", packets)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			packet := gsoTestPacket(test.v6, true, 701, 250)
			ipSize := 20
			if test.v6 {
				ipSize = 40
			}
			headers := 14 + ipSize + 32
			wantPayload := bytes.Clone(packet[12+headers:])
			var lengths []int
			var payload []byte
			if err := state.normalizePeerPackets([][]byte{packet}, buffers, func(packets [][]byte) error {
				for _, packet := range packets {
					if !bytes.Equal(packet[:12], make([]byte, 12)) {
						t.Fatalf("nonzero output Virtio header: %x", packet[:12])
					}
					frame := packet[12:]
					ip, tcp := frame[14:], frame[14+ipSize:]
					if binary.BigEndian.Uint32(tcp[4:]) != uint32(0xfffffff0)+uint32(len(payload)) {
						t.Fatal("TCP segment sequence changed")
					}
					flags := byte(0x50)
					if len(payload) == 0 {
						flags |= 0x80
					}
					if len(payload)+len(frame)-headers == 701 {
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
						t.Fatal("invalid TCP checksum")
					}
					lengths = append(lengths, len(frame)-headers)
					payload = append(payload, frame[headers:]...)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(lengths, []int{188, 188, 188, 137}) || !bytes.Equal(payload, wantPayload) {
				t.Fatalf("TCP payload lengths = %v, payload equal = %v", lengths, bytes.Equal(payload, wantPayload))
			}
		})
	}
}

func TestPeerVirtioNormalizationBatches(t *testing.T) {
	state := gsoTestState(t, false, 28)
	packet := gsoTestPacket(false, false, 2001, 250)
	for i := 12 + 66; i < len(packet); i++ {
		packet[i] = byte(i / 31)
	}
	want := bytes.Clone(packet[12+66:])
	buffers := make([][]byte, 32)
	for i := range buffers {
		buffers[i] = bytes.Repeat([]byte{0xa5}, 1526)
	}
	var sizes []int
	var payload []byte
	err := state.normalizePeerPackets([][]byte{packet}, buffers, func(packets [][]byte) error {
		sizes = append(sizes, len(packets))
		batchBytes := 0
		for _, packet := range packets {
			batchBytes += len(packet)
			if !bytes.Equal(packet[:12], make([]byte, 12)) {
				t.Fatal("stale output Virtio header")
			}
			if binary.BigEndian.Uint32(packet[12+38:]) != uint32(0xfffffff0)+uint32(len(payload)) {
				t.Fatal("borrowed segment was overwritten before the batch write")
			}
			payload = append(payload, packet[12+66:]...)
		}
		if len(packets) > 32 || batchBytes > 256000 {
			t.Fatalf("oversized batch: %d packets, %d bytes", len(packets), batchBytes)
		}
		return nil
	})
	if err != nil || !slices.Equal(sizes, []int{32, 32, 32, 30}) || !bytes.Equal(payload, want) {
		t.Fatalf("normalization = %v, batches = %v, payload equal = %v", err, sizes, bytes.Equal(payload, want))
	}
	calls := 0
	err = state.normalizePeerPackets([][]byte{packet}, buffers, func([][]byte) error {
		calls++
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("interrupted conversion = %v, writes = %d", err, calls)
	}
}
