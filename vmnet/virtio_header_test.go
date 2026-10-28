package vmnet

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestInterfaceVirtioHeaderPackets(t *testing.T) {
	plain, header := batchInterfaces(t,
		WithInterfaceChecksumOffload(true), WithInterfaceChecksumOffload(false),
		WithInterfaceVirtioHeader(false), WithInterfaceVirtioHeader(true),
	)
	if plain.MaxPacketSize() != 1514 || header.MaxPacketSize() != 1526 {
		t.Fatalf("maximum packet sizes = %d, %d, want 1514, 1526", plain.MaxPacketSize(), header.MaxPacketSize())
	}
	for _, tt := range []struct {
		name             string
		sender, receiver *Interface
		writeHeader      bool
	}{
		{"plain to header", plain, header, false},
		{"header to plain", header, plain, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			frames := [][]byte{
				batchFrame(tt.sender.MACAddress(), 0, 60),
				batchFrame(tt.sender.MACAddress(), 1, 1514),
				batchFrame(tt.sender.MACAddress(), 2, 90),
			}
			packets := make([][]byte, len(frames))
			for j, frame := range frames {
				packets[j] = frame
				if tt.writeHeader {
					packets[j] = append(make([]byte, 12), frame...)
				}
			}
			if err := tt.sender.WritePacket(packets[0]); err != nil {
				t.Fatal(err)
			}
			if n, err := tt.sender.WritePackets(packets[1:]); err != nil || n != 2 {
				t.Fatalf("write batch = %d, %v", n, err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			buffers := [][]byte{make([]byte, tt.receiver.MaxPacketSize()), make([]byte, tt.receiver.MaxPacketSize())}
			received := 0
			for received < len(frames) {
				var n int
				var err error
				if received == 0 {
					var size int
					size, err = tt.receiver.ReadPacket(ctx, buffers[0])
					buffers[0] = buffers[0][:size]
					n = 1
				} else {
					n, err = tt.receiver.ReadPackets(ctx, buffers)
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, packet := range buffers[:n] {
					offset := 0
					if !tt.writeHeader {
						offset = 12
					}
					if len(packet) < offset+18 || binary.BigEndian.Uint16(packet[offset+12:]) != 0x88b5 {
						continue
					}
					if received >= len(frames) || !bytes.Equal(packet[offset:], frames[received]) {
						t.Fatalf("packet %d differs: %x", received, packet)
					}
					received++
				}
				buffers[0] = buffers[0][:cap(buffers[0])]
			}
		})
	}
}

func TestInterfaceVirtioHeaderChecksumConflict(t *testing.T) {
	if Available(26) {
		t.Skip("requires macOS 26 SDK")
	}
	for _, opts := range [][]InterfaceOption{
		{WithInterfaceVirtioHeader(true), WithInterfaceChecksumOffload(true)},
		{WithInterfaceChecksumOffload(true), WithInterfaceVirtioHeader(true)},
	} {
		iface, err := StartBridgedInterface("vmnet-option-test", opts...)
		if iface != nil {
			iface.Stop()
		}
		if iface != nil || err == nil || !strings.Contains(err.Error(), "virtio header and checksum offload") {
			t.Fatalf("conflicting options = %v, %v", iface, err)
		}
	}
}
