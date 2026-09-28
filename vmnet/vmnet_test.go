package vmnet_test

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/osversion"
	"github.com/Code-Hex/vz/v3/vmnet"
)

func TestNetworkIPv4Subnet(t *testing.T) {
	if err := osversion.MacOSAvailable(26); err != nil {
		t.Skipf("vmnet network requires macOS 26: %v", err)
	}

	config, err := vmnet.NewNetworkConfiguration(vmnet.HostMode)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	subnet, err := network.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	if !subnet.IsValid() || !subnet.Addr().Is4() {
		t.Fatalf("expected an IPv4 subnet, got %s", subnet)
	}
}
