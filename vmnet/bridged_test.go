package vmnet_test

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestBridgedInterfaceInvalidName(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet interfaces require macOS 26 and its SDK")
	}
	for _, name := range []string{"", "en0\x00ignored"} {
		t.Run(name, func(t *testing.T) {
			iface, err := vmnet.StartBridgedInterface(name)
			if iface != nil || err == nil || !strings.Contains(err.Error(), "interface name") {
				t.Fatalf("StartBridgedInterface(%q) = %v, %v", name, iface, err)
			}
		})
	}
}

func TestBridgedInterfaces(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet interfaces require macOS 26 and its SDK")
	}
	names, err := vmnet.BridgedInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == "" || strings.ContainsRune(name, 0) {
			t.Fatalf("invalid interface name %q", name)
		}
	}
}

func TestBridgedInterfaceLifecycle(t *testing.T) {
	name := os.Getenv("VZ_VMNET_BRIDGED_INTERFACE")
	if name == "" {
		t.Skip("set VZ_VMNET_BRIDGED_INTERFACE to a physical interface to enable this test")
	}
	iface, err := vmnet.StartBridgedInterface(name)
	if err != nil {
		t.Fatal(err)
	}
	defer iface.Stop()
	if iface.MaxPacketSize() <= 0 || len(iface.MACAddress()) != 6 {
		t.Fatalf("interface metadata: size %d, MAC %v", iface.MaxPacketSize(), iface.MACAddress())
	}
	if err := iface.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := iface.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := iface.ReadPacket(context.Background(), make([]byte, iface.MaxPacketSize())); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("read after stop = %v", err)
	}
}
