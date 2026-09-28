package datagram_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3/internal/osversion"
	"github.com/Code-Hex/vz/v3/vmnet"
	"github.com/Code-Hex/vz/v3/vmnet/fileadapter/datagram"
)

func TestFileAdapterForInterface(t *testing.T) {
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
	adapter, err := vmnet.StartInterfaceWithNetwork(network, nil)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := vmnet.StartInterfaceWithNetwork(network, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Stop() })
	ctx, cancel := context.WithCancel(context.Background())
	file, run, err := datagram.FileAdapterForInterface(ctx, adapter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = file.Close()
	})
	result := make(chan error, 1)
	go func() { result <- run() }()

	packet := make([]byte, 60)
	for index := range 6 {
		packet[index] = 0xff
	}
	packet[6] = 0x02
	packet[12], packet[13] = 0x88, 0xb5
	copy(packet[14:], []byte("vmnet datagram adapter"))

	peerReady := make(chan struct{}, 1)
	if err := peer.SetPacketsAvailableEventCallback(func(int) {
		select {
		case peerReady <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.SetPacketsAvailableEventCallback(nil) })
	if _, err := file.Write(packet); err != nil {
		t.Fatal(err)
	}
	manager, err := vmnet.NewPktDescsManager(1, peer.MaxPacketSize)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
peerLoop:
	for {
		select {
		case <-peerReady:
		case <-deadline:
			t.Fatal("peer did not receive device packet")
		}
		for {
			count, err := peer.ReadPackets(manager, 1)
			if err != nil {
				t.Fatal(err)
			}
			if count == 0 {
				break
			}
			got, err := manager.Packet(0)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(got, packet) {
				break peerLoop
			}
		}
	}

	if err := manager.SetPacket(0, packet); err != nil {
		t.Fatal(err)
	}
	if err := peer.WritePackets(manager, 1); err != nil {
		t.Fatal(err)
	}
	if err := file.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, int(adapter.MaxPacketSize)+1)
	for {
		n, err := file.Read(buffer)
		if err != nil {
			t.Fatalf("read device packet: %v", err)
		}
		if bytes.Equal(buffer[:n], packet) {
			break
		}
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("adapter did not stop after cancellation")
	}
	if err := run(); err != nil {
		t.Fatalf("second Run returned %v", err)
	}
}

func TestFileAdapterForInterfaceRejectsNil(t *testing.T) {
	if _, _, err := datagram.FileAdapterForInterface(context.Background(), nil); err == nil {
		t.Fatal("nil interface must fail")
	}
	if _, _, err := datagram.FileAdapterForInterface(nil, &vmnet.Interface{}); err == nil {
		t.Fatal("nil context must fail")
	}
}
