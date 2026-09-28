package vmnet_test

import (
	"math"
	"testing"

	"github.com/Code-Hex/vz/v3/vmnet"
)

func TestPktDescsManager(t *testing.T) {
	manager, err := vmnet.NewPktDescsManager(2, 8)
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.MaxPacketCount(); got != 2 {
		t.Fatalf("packet count = %d, want 2", got)
	}
	if _, err := manager.Packet(0); err == nil {
		t.Fatal("uninitialized packet must fail")
	}
	if err := manager.SetPacket(0, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	packet, err := manager.Packet(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 3 || packet[0] != 1 || packet[1] != 2 || packet[2] != 3 {
		t.Fatalf("packet = %v, want [1 2 3]", packet)
	}
	packet[0] = 9
	packet, err = manager.Packet(0)
	if err != nil || packet[0] != 1 {
		t.Fatalf("packet must be copied: %v, %v", packet, err)
	}
	for _, packet := range [][]byte{nil, make([]byte, 9)} {
		if err := manager.SetPacket(0, packet); err == nil {
			t.Fatalf("packet size %d must fail", len(packet))
		}
	}
	for _, index := range []int{-1, 2} {
		if err := manager.SetPacket(index, []byte{1}); err == nil {
			t.Fatalf("SetPacket(%d) must fail", index)
		}
		if _, err := manager.Packet(index); err == nil {
			t.Fatalf("Packet(%d) must fail", index)
		}
	}
}

func TestPktDescsManagerAllocationBounds(t *testing.T) {
	for _, input := range []struct {
		count int
		size  uint64
	}{
		{0, 1},
		{-1, 1},
		{1, 0},
		{math.MaxInt32 + 1, 1},
		{2, math.MaxUint64},
		{2, math.MaxInt},
	} {
		if _, err := vmnet.NewPktDescsManager(input.count, input.size); err == nil {
			t.Fatalf("NewPktDescsManager(%d, %d) must fail", input.count, input.size)
		}
	}
}

func TestInterfacePacketRequestBounds(t *testing.T) {
	manager, err := vmnet.NewPktDescsManager(2, 8)
	if err != nil {
		t.Fatal(err)
	}
	var absent *vmnet.Interface
	if _, err := absent.ReadPackets(manager, 1); err == nil {
		t.Fatal("nil interface read must fail")
	}
	if err := absent.WritePackets(manager, 1); err == nil {
		t.Fatal("nil interface write must fail")
	}
	iface := &vmnet.Interface{MaxPacketSize: 8, MaxReadPacketCount: 2, MaxWritePacketCount: 2}
	for _, count := range []int{-1, 0, 3} {
		if _, err := iface.ReadPackets(manager, count); err == nil {
			t.Fatalf("read count %d must fail", count)
		}
		if err := iface.WritePackets(manager, count); err == nil {
			t.Fatalf("write count %d must fail", count)
		}
	}
}
