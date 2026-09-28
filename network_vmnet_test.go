package vz_test

import (
	"testing"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/osversion"
	"github.com/Code-Hex/vz/v3/vmnet"
)

func TestVmnetNetworkDeviceAttachment(t *testing.T) {
	if err := osversion.MacOSAvailable(26); err != nil {
		t.Skipf("vmnet attachment requires macOS 26: %v", err)
	}

	config, err := vmnet.NewNetworkConfiguration(vmnet.HostMode)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := vz.NewVmnetNetworkDeviceAttachment(network.Raw())
	if err != nil {
		t.Fatal(err)
	}
	if attachment == nil {
		t.Fatal("expected vmnet attachment")
	}
	if got := attachment.Network(); got != network.Raw() {
		t.Fatalf("attachment.Network() = %p, want %p", got, network.Raw())
	}
}
