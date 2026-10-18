package vmnet_test

import (
	"net/netip"
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetCopiedConfigurationAndNetwork(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, collectedConfig := func() (vmnet.NetworkConfiguration, <-chan struct{}) {
		original, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.251.0/24")))
		if err != nil {
			t.Fatal(err)
		}
		collected := make(chan struct{})
		runtime.AddCleanup(original, func(ch chan struct{}) { close(ch) }, collected)
		return *original, collected
	}()
	awaitNativeBridge(t, func() bool {
		select {
		case <-collectedConfig:
			return true
		default:
			return false
		}
	})
	network, collectedNetwork := func() (vmnet.Network, <-chan struct{}) {
		original, err := vmnet.NewNetwork(&config)
		if err != nil {
			t.Fatal(err)
		}
		collected := make(chan struct{})
		runtime.AddCleanup(original, func(ch chan struct{}) { close(ch) }, collected)
		return *original, collected
	}()
	awaitNativeBridge(t, func() bool {
		select {
		case <-collectedNetwork:
			return true
		default:
			return false
		}
	})
	if got, err := network.IPv4Subnet(); err != nil || got != netip.MustParsePrefix("192.168.251.0/24") {
		t.Fatalf("copied network subnet = %v, %v", got, err)
	}
	runtime.KeepAlive(config)
	runtime.KeepAlive(network)
}

func awaitNativeBridge(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		vzbridge.Drain()
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("native cleanup did not complete within 10 seconds")
}
