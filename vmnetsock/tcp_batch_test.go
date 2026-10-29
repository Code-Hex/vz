package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"slices"
	"testing"
	"time"
)

func TestTCPBatchCopiesSegments(t *testing.T) {
	peer := netip.MustParseAddrPort("192.0.2.2:1000")
	remote := netip.MustParseAddrPort("192.0.2.1:80")
	now := time.Now()
	state := &adapterState{tcp: newTCPSegmentation(1500)}
	if err := state.tcp.observePeer(flowPacket(peer, remote, 2, 10, 0, 256, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := state.tcp.forwardNative(flowPacket(remote, peer, 0x12, 20, 11, 1460, 0), now, func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}

	frame := flowPacket(remote, peer, 0x18, 21, 11, 0, 1200)
	payload := frame[54:]
	for i := range payload {
		payload[i] = byte(i / 31)
	}
	packet := append(make([]byte, virtioHeaderSize), frame...)
	peerFrames := [][]byte{make([]byte, 1514), make([]byte, 1514)}
	var batchSizes []int
	var received []byte
	err := state.forwardTCPPackets([][]byte{packet}, peerFrames, func(frames [][]byte) error {
		batchSizes = append(batchSizes, len(frames))
		for _, frame := range frames {
			if len(frame) < 54 {
				t.Fatalf("short TCP frame: %d bytes", len(frame))
			}
			if got := binary.BigEndian.Uint32(frame[38:]); got != 21+uint32(len(received)) {
				t.Fatalf("segment sequence = %d, want %d", got, 21+len(received))
			}
			received = append(received, frame[54:]...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(batchSizes, []int{2, 2, 1}) {
		t.Fatalf("batch sizes = %v, want [2 2 1]", batchSizes)
	}
	if !bytes.Equal(received, payload) {
		t.Fatal("batching changed the TCP payload")
	}
}
