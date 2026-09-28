package vz_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/objc"
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
	attachment, err := vz.NewVmnetNetworkDeviceAttachment(network)
	if err != nil {
		t.Fatal(err)
	}
	if attachment == nil {
		t.Fatal("expected vmnet attachment")
	}
	if got := attachment.Network(); objc.Ptr(got) != objc.Ptr(network) {
		t.Fatalf("attachment.Network() = %p, want %p", objc.Ptr(got), objc.Ptr(network))
	}
}

func TestVmnetNetworkDeviceAttachmentNetworkOutlivesAttachment(t *testing.T) {
	if err := osversion.MacOSAvailable(26); err != nil {
		t.Skipf("vmnet attachment requires macOS 26: %v", err)
	}

	collected := make(chan struct{})
	borrowed := func() *vmnet.Network {
		config, err := vmnet.NewNetworkConfiguration(vmnet.HostMode)
		if err != nil {
			t.Fatal(err)
		}
		network, err := vmnet.NewNetwork(config)
		if err != nil {
			t.Fatal(err)
		}
		attachment, err := vz.NewVmnetNetworkDeviceAttachment(network)
		if err != nil {
			t.Fatal(err)
		}
		runtime.AddCleanup(attachment, func(done chan struct{}) { close(done) }, collected)
		return attachment.Network()
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
waitForCollection:
	for {
		runtime.GC()
		select {
		case <-collected:
			break waitForCollection
		case <-deadline.C:
			t.Fatal("attachment was not collected")
		case <-time.After(10 * time.Millisecond):
		}
	}
	subnet, err := borrowed.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	if !subnet.IsValid() || !subnet.Addr().Is4() {
		t.Fatalf("expected an IPv4 subnet after attachment collection, got %s", subnet)
	}
}
