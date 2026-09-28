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
	if err := iface.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartInterfaceWithNilNetwork(t *testing.T) {
	if _, err := vmnet.StartInterfaceWithNetwork(nil, nil); err == nil {
		t.Fatal("expected an error for a nil network")
	}
}
