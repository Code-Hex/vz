package vmnetsock_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnetsock"
)

func TestPeerVirtioNormalizationRequiresVirtioHeader(t *testing.T) {
	network := newShutdownNetwork(t)
	iface := newInterface(t, network)
	adapter, err := vmnetsock.New(t.Context(), iface, vmnetsock.Stream, vmnetsock.WithPeerVirtioHeaderNormalization())
	if err == nil {
		adapter.Close()
		t.Fatal("peer normalization accepted an interface without Virtio headers")
	}
}

func TestPeerVirtioNormalizationChecksumWithoutTSO(t *testing.T) {
	for _, transport := range []vmnetsock.Transport{vmnetsock.Stream, vmnetsock.Datagram} {
		t.Run(fmt.Sprint(transport), func(t *testing.T) {
			network := newShutdownNetwork(t)
			iface := newInterface(t, network, vmnet.WithInterfaceVirtioHeader(true))
			receiver := newInterface(t, network)
			adapter, err := vmnetsock.New(t.Context(), iface, transport, vmnetsock.WithPeerVirtioHeaderNormalization())
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			peer, err := net.FileConn(duplicate(t, adapter))
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}

			frame := shutdownFrame(iface, 0)
			copy(frame[14:], []byte{0x12, 0x34, 0x56, 0x78, 0x9a})
			packet := append(make([]byte, 12), frame...)
			packet[0] = 1
			binary.LittleEndian.PutUint16(packet[6:], 14)
			writeNormalizationPacket(t, peer, transport, packet)
			want := bytes.Clone(frame)
			copy(want[14:], []byte{0xfd, 0x52})
			readShutdownFrame(t, receiver, want)
			if err := adapter.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPeerVirtioNormalizationRejectsRSC(t *testing.T) {
	for _, flags := range []byte{4, 6} {
		t.Run(fmt.Sprint(flags), func(t *testing.T) {
			network := newShutdownNetwork(t)
			iface := newInterface(t, network, vmnet.WithInterfaceVirtioHeader(true))
			adapter, err := vmnetsock.New(context.Background(), iface, vmnetsock.Stream, vmnetsock.WithPeerVirtioHeaderNormalization())
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			peer, err := net.FileConn(duplicate(t, adapter))
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			packet := append(make([]byte, 12), shutdownFrame(iface, 1)...)
			packet[0] = flags
			writeNormalizationPacket(t, peer, vmnetsock.Stream, packet)
			if _, err := io.Copy(io.Discard, peer); err != nil {
				t.Fatalf("RSC input did not close the socket: %v", err)
			}
			if err := adapter.Close(); err == nil || !strings.Contains(err.Error(), "RSC") {
				t.Fatalf("Close = %v, want an RSC error", err)
			}
		})
	}
}

func TestPeerVirtioNormalizationOptionOrder(t *testing.T) {
	for _, test := range []struct {
		name       string
		opts       []vmnetsock.Option
		peerHeader bool
	}{
		{
			name:       "normalization last",
			opts:       []vmnetsock.Option{vmnetsock.WithVirtioHeaderConversion(), vmnetsock.WithPeerVirtioHeaderNormalization()},
			peerHeader: true,
		},
		{
			name: "Ethernet conversion last",
			opts: []vmnetsock.Option{vmnetsock.WithPeerVirtioHeaderNormalization(), vmnetsock.WithVirtioHeaderConversion()},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			network := newShutdownNetwork(t)
			iface := newInterface(t, network, vmnet.WithInterfaceVirtioHeader(true))
			receiver := newInterface(t, network)
			adapter, err := vmnetsock.New(t.Context(), iface, vmnetsock.Stream, test.opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			peer, err := net.FileConn(duplicate(t, adapter))
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			frame := shutdownFrame(iface, 1)
			packet := frame
			if test.peerHeader {
				packet = append(make([]byte, 12), packet...)
			}
			writeNormalizationPacket(t, peer, vmnetsock.Stream, packet)
			readShutdownFrame(t, receiver, frame)
			if err := adapter.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func writeNormalizationPacket(t *testing.T, peer net.Conn, transport vmnetsock.Transport, packet []byte) {
	t.Helper()
	if transport == vmnetsock.Stream {
		packet = append(binary.BigEndian.AppendUint32(nil, uint32(len(packet))), packet...)
	}
	if _, err := peer.Write(packet); err != nil {
		t.Fatal(err)
	}
}
