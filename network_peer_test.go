package vz_test

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"golang.org/x/sys/unix"
)

type networkPeer struct {
	attachment *vz.FileHandleNetworkDeviceAttachment
	ip         netip.Addr
	done       chan error
}

func newNetworkPeer(t *testing.T, ip netip.Addr, mac [6]byte) *networkPeer {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	deviceFile := os.NewFile(uintptr(fds[0]), "network-device")
	defer deviceFile.Close()
	peerFile := os.NewFile(uintptr(fds[1]), "network-peer")
	defer peerFile.Close()

	attachment, err := vz.NewFileHandleNetworkDeviceAttachment(deviceFile)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.FileConn(peerFile)
	if err != nil {
		t.Fatal(err)
	}
	peer := &networkPeer{attachment: attachment, ip: ip, done: make(chan error, 1)}
	go func() {
		peer.done <- serveNetworkPeer(conn, ip, mac)
	}()
	t.Cleanup(func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close network peer: %v", err)
		}
		select {
		case err := <-peer.done:
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("network peer stopped: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("network peer did not stop")
		}
	})
	return peer
}

func serveNetworkPeer(conn net.Conn, ip netip.Addr, mac [6]byte) error {
	buffer := make([]byte, 65536)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return err
		}
		reply := arpReply(buffer[:n], ip, mac)
		if reply == nil {
			reply = echoReply(buffer[:n], ip, mac)
		}
		if reply == nil {
			continue
		}
		if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		written, err := conn.Write(reply)
		if err != nil {
			return err
		}
		if written != len(reply) {
			return io.ErrShortWrite
		}
	}
}

func checkNetworkPeer(t *testing.T, peer *networkPeer) {
	t.Helper()
	select {
	case err := <-peer.done:
		peer.done <- err
		t.Fatalf("network peer stopped before cleanup: %v", err)
	default:
	}
}
