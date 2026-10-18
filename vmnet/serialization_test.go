package vmnet_test

import (
	"net/netip"
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestNetworkSerialization(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	want := netip.MustParsePrefix("192.168.249.0/24")
	if err := config.SetIPv4Subnet(want); err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	alias, collected := func() (vmnet.NetworkSerialization, <-chan struct{}) {
		serialization, err := network.CopySerialization()
		if err != nil {
			t.Fatal(err)
		}
		collected := make(chan struct{})
		runtime.AddCleanup(serialization, func(ch chan struct{}) { close(ch) }, collected)
		return *serialization, collected
	}()
	awaitNativeBridge(t, func() bool {
		select {
		case <-collected:
			return true
		default:
			return false
		}
	})
	restored, err := vmnet.NewNetworkWithSerialization(&alias)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("subnet = %v, want %v", got, want)
	}
}

func TestNetworkSerializationInvalidInputs(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, network := range []*vmnet.Network{nil, {}} {
		if _, err := network.CopySerialization(); err == nil {
			t.Fatal("invalid network accepted")
		}
	}
	for _, serialization := range []*vmnet.NetworkSerialization{nil, {}} {
		if _, err := vmnet.NewNetworkWithSerialization(serialization); err == nil {
			t.Fatal("invalid serialization accepted")
		}
	}
}
