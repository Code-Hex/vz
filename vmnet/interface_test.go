package vmnet_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func interfaceNetwork(t *testing.T) *vmnet.Network {
	t.Helper()
	if vmnet.Available(26) {
		t.Skip("vmnet interfaces require macOS 26 and its SDK")
	}
	c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	n, err := vmnet.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func startInterface(t *testing.T, n *vmnet.Network, opts ...vmnet.InterfaceOption) *vmnet.Interface {
	t.Helper()
	i, err := vmnet.StartInterfaceWithNetwork(n, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := i.Stop(); err != nil {
			t.Error(err)
		}
	})
	return i
}

func TestVMNetInterfacePackets(t *testing.T) {
	n := interfaceNetwork(t)
	a := startInterface(t, n)
	b := startInterface(t, n)
	frame, _ := hex.DecodeString("ffffffffffff02000000000108060001080006040001020000000001c0a8ff02000000000000c0a8ff03000000000000000000000000000000000000")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.WritePacket(frame); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, b.MaxPacketSize())
	for {
		count, err := b.ReadPacket(ctx, buf)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(buf[:count], frame) {
			break
		}
	}
	if len(a.MACAddress()) != 6 {
		t.Fatalf("MAC = %v", a.MACAddress())
	}
	mac := a.MACAddress()
	mac[0] ^= 0xff
	if bytes.Equal(mac, a.MACAddress()) {
		t.Fatal("MAC getter exposes mutable storage")
	}
}

func TestVMNetInterfaceBoundaries(t *testing.T) {
	for _, n := range []*vmnet.Network{nil, {}} {
		if i, err := vmnet.StartInterfaceWithNetwork(n); err == nil || i != nil {
			t.Fatal("invalid network accepted")
		}
	}
	for _, i := range []*vmnet.Interface{nil, {}} {
		if _, err := i.ReadPacket(context.Background(), make([]byte, 100)); err == nil {
			t.Fatal("invalid interface read")
		}
		if err := i.WritePacket([]byte{1}); err == nil {
			t.Fatal("invalid interface write")
		}
		if err := i.Stop(); err == nil {
			t.Fatal("invalid interface stopped")
		}
		if i.MaxPacketSize() != 0 || i.MACAddress() != nil || i.VirtioHeaderEnabled() || i.TSOEnabled() {
			t.Fatal("invalid interface metadata")
		}
	}
	i := startInterface(t, interfaceNetwork(t))
	if err := i.WritePacket(nil); err == nil {
		t.Fatal("empty write accepted")
	}
	if err := i.WritePacket(make([]byte, i.MaxPacketSize()+1)); err == nil {
		t.Fatal("oversized write accepted")
	}
	if _, err := i.ReadPacket(context.Background(), make([]byte, i.MaxPacketSize()-1)); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("short read = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := i.ReadPacket(ctx, make([]byte, i.MaxPacketSize())); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}
}

func TestVMNetInterfaceStopAliasesAndReuse(t *testing.T) {
	n := interfaceNetwork(t)
	original := startInterface(t, n)
	copied := *original
	runtime.GC()
	runtime.KeepAlive(original)
	done := make(chan error, 1)
	go func() {
		for {
			_, err := copied.ReadPacket(context.Background(), make([]byte, copied.MaxPacketSize()))
			if err != nil {
				done <- err
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := copied.Stop(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Errorf("stopped read = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not unblock read")
	}
	if err := original.WritePacket([]byte{1}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after stop = %v", err)
	}
	startInterface(t, n, vmnet.WithInterfaceChecksumOffload(true), vmnet.WithInterfaceTSO(true))
	startInterface(t, n, vmnet.WithInterfaceMACAllocation(false), vmnet.WithInterfaceIsolation(true))
}

func TestVMNetInterfaceFunctionalOptions(t *testing.T) {
	n := interfaceNetwork(t)
	for _, tt := range []struct {
		name     string
		options  []vmnet.InterfaceOption
		checkMAC bool
	}{
		{name: "defaults", checkMAC: true},
		{
			name: "disable MAC allocation",
			options: []vmnet.InterfaceOption{
				vmnet.WithInterfaceMACAllocation(false),
				vmnet.WithInterfaceIsolation(true),
			},
		},
		{
			name: "last option wins",
			options: []vmnet.InterfaceOption{
				vmnet.WithInterfaceMACAllocation(false),
				vmnet.WithInterfaceMACAllocation(true),
				vmnet.WithInterfaceChecksumOffload(true),
				vmnet.WithInterfaceChecksumOffload(false),
				vmnet.WithInterfaceTSO(true),
				vmnet.WithInterfaceTSO(false),
				vmnet.WithInterfaceIsolation(true),
				vmnet.WithInterfaceIsolation(false),
			},
			checkMAC: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i, err := vmnet.StartInterfaceWithNetwork(n, tt.options...)
			if err != nil {
				t.Fatal(err)
			}
			defer i.Stop()
			if i.MaxPacketSize() <= 0 {
				t.Fatalf("maximum packet size = %d", i.MaxPacketSize())
			}
			if tt.checkMAC && len(i.MACAddress()) != 6 {
				t.Fatalf("MAC address = %v", i.MACAddress())
			}
		})
	}
}

func TestVMNetInterfaceCollectedWrappers(t *testing.T) {
	copied, peer, collectedInterface, collectedNetwork := func() (vmnet.Interface, *vmnet.Interface, <-chan struct{}, <-chan struct{}) {
		n := interfaceNetwork(t)
		i, err := vmnet.StartInterfaceWithNetwork(n)
		if err != nil {
			t.Fatal(err)
		}
		peer, err := vmnet.StartInterfaceWithNetwork(n)
		if err != nil {
			t.Fatal(err)
		}
		ci, cn := make(chan struct{}), make(chan struct{})
		runtime.AddCleanup(i, func(ch chan struct{}) { close(ch) }, ci)
		runtime.AddCleanup(n, func(ch chan struct{}) { close(ch) }, cn)
		return *i, peer, ci, cn
	}()
	defer copied.Stop()
	defer peer.Stop()
	for _, collected := range []<-chan struct{}{collectedInterface, collectedNetwork} {
		awaitNativeBridge(t, func() bool {
			select {
			case <-collected:
				return true
			default:
				return false
			}
		})
	}
	frame, _ := hex.DecodeString("ffffffffffff02000000000108060001080006040001020000000001c0a8ff02000000000000c0a8ff03000000000000000000000000000000000000")
	if err := copied.WritePacket(frame); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	buffer := make([]byte, peer.MaxPacketSize())
	for {
		n, err := peer.ReadPacket(ctx, buffer)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(buffer[:n], frame) {
			break
		}
	}
}
