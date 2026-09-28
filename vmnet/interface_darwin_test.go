package vmnet_test

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/osversion"
	"github.com/Code-Hex/vz/v3/vmnet"
)

func TestInterfaceStartAndStop(t *testing.T) {
	if err := osversion.MacOSAvailable(26); err != nil {
		t.Skipf("vmnet interface requires macOS 26: %v", err)
	}

	config, err := vmnet.NewNetworkConfiguration(vmnet.HostMode)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	iface, err := vmnet.StartInterfaceWithNetwork(network, nil)
	if err != nil {
		t.Fatal(err)
	}
	if iface.MaxPacketSize == 0 || iface.MaxReadPacketCount == 0 || iface.MaxWritePacketCount == 0 {
		t.Errorf("invalid packet limits: size=%d read=%d write=%d", iface.MaxPacketSize, iface.MaxReadPacketCount, iface.MaxWritePacketCount)
	}
	manager, err := vmnet.NewPktDescsManager(1, iface.MaxPacketSize)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{0, 2} {
		if _, err := iface.ReadPackets(manager, count); err == nil {
			t.Fatalf("ReadPackets with count %d must fail", count)
		}
		if err := iface.WritePackets(manager, count); err == nil {
			t.Fatalf("WritePackets with count %d must fail", count)
		}
	}
	oversized, err := vmnet.NewPktDescsManager(1, iface.MaxPacketSize+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iface.ReadPackets(oversized, 1); err == nil {
		t.Fatal("ReadPackets with oversized buffer must fail")
	}
	if err := iface.WritePackets(oversized, 1); err == nil {
		t.Fatal("WritePackets with oversized buffer must fail")
	}
	undersized, err := vmnet.NewPktDescsManager(1, iface.MaxPacketSize-1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iface.ReadPackets(undersized, 1); err == nil {
		t.Fatal("ReadPackets with undersized buffer must fail")
	}
	if _, err := iface.ReadPackets(manager, 1); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 60)
	for index := range 6 {
		packet[index] = 0xff
	}
	packet[6] = 0x02
	packet[12], packet[13] = 0x08, 0x06
	if err := manager.SetPacket(0, packet); err != nil {
		t.Fatal(err)
	}
	if err := iface.WritePackets(manager, 1); err != nil {
		t.Fatal(err)
	}
	if err := iface.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartInterfaceWithNilNetwork(t *testing.T) {
	if _, err := vmnet.StartInterfaceWithNetwork(nil, nil); err == nil {
		t.Fatal("expected an error for a nil network")
	}
}
