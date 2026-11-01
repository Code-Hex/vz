package vz_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"golang.org/x/sys/unix"
)

type icmpPeer struct {
	attachment *vz.FileHandleNetworkDeviceAttachment
	ip         netip.Addr
	mac        [6]byte
	done       chan error
}

func newICMPPeer(t *testing.T, ip netip.Addr, mac [6]byte) *icmpPeer {
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
	peer := &icmpPeer{attachment: attachment, ip: ip, mac: mac, done: make(chan error, 1)}
	go func() {
		peer.done <- serveICMP(conn, ip, mac)
	}()
	t.Cleanup(func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close ICMP peer: %v", err)
		}
		select {
		case err := <-peer.done:
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("ICMP peer stopped: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("ICMP peer did not stop")
		}
	})
	return peer
}

func serveICMP(conn net.Conn, ip netip.Addr, mac [6]byte) error {
	buffer := make([]byte, 65536)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return err
		}
		reply := echoReply(buffer[:n], ip, mac)
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

func checkICMPPeer(t *testing.T, peer *icmpPeer) {
	t.Helper()
	select {
	case err := <-peer.done:
		peer.done <- err
		t.Fatalf("ICMP peer stopped before cleanup: %v", err)
	default:
	}
}

func (p *icmpPeer) staticARP() string {
	return fmt.Sprintf("arp -i eth1 -s %s %02x:%02x:%02x:%02x:%02x:%02x", p.ip,
		p.mac[0], p.mac[1], p.mac[2], p.mac[3], p.mac[4], p.mac[5])
}
