package vmnetsock_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnetsock"
)

func TestAdapterCloseIncompleteStream(t *testing.T) {
	for _, test := range []struct {
		name   string
		suffix []byte
		race   bool
	}{
		{
			name:   "header",
			suffix: []byte{0, 0},
		},
		{
			name:   "body",
			suffix: []byte{0, 0, 0, 60, 1},
		},
		{
			name:   "header cancellation",
			suffix: []byte{0, 0},
			race:   true,
		},
		{
			name:   "body cancellation",
			suffix: []byte{0, 0, 0, 60, 1},
			race:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			network := newShutdownNetwork(t)
			sender, receiver := newInterface(t, network), newInterface(t, network)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			adapter, err := vmnetsock.New(ctx, sender, vmnetsock.Stream)
			if err != nil {
				t.Fatal(err)
			}
			file := duplicate(t, adapter)
			peer, err := net.FileConn(file)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}

			frame := shutdownFrame(sender, 1)
			wire := binary.BigEndian.AppendUint32(nil, uint32(len(frame)))
			wire = append(wire, frame...)
			wire = append(wire, test.suffix...)
			if _, err := peer.Write(wire); err != nil {
				t.Fatal(err)
			}
			readShutdownFrame(t, receiver, frame)

			closed := make(chan error, 1)
			go func() {
				closed <- adapter.Close()
			}()
			if test.race {
				cancel()
			}
			select {
			case err = <-closed:
				if err != nil && (!test.race || !errors.Is(err, context.Canceled)) {
					t.Fatalf("Close = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Close did not interrupt an incomplete frame")
			}
			if next := adapter.Close(); next != err {
				t.Fatalf("repeated Close = %v, first Close = %v", next, err)
			}
			if _, err := file.Stat(); err != nil {
				t.Fatalf("Close invalidated the caller's descriptor: %v", err)
			}
			frame = shutdownFrame(sender, 2)
			if err := sender.WritePacket(frame); err != nil {
				t.Fatalf("borrowed interface write after Close: %v", err)
			}
			readShutdownFrame(t, receiver, frame)
		})
	}
}

func TestAdapterStreamPrefixBeforeError(t *testing.T) {
	for _, test := range []struct {
		name   string
		suffix []byte
	}{
		{
			name:   "invalid length",
			suffix: []byte{0, 0, 0, 0},
		},
		{
			name:   "truncated header",
			suffix: []byte{0, 0},
		},
		{
			name:   "truncated body",
			suffix: []byte{0, 0, 0, 60, 1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			network := newShutdownNetwork(t)
			sender, receiver := newInterface(t, network), newInterface(t, network)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			adapter, err := vmnetsock.New(ctx, sender, vmnetsock.Stream)
			if err != nil {
				t.Fatal(err)
			}
			peer, err := net.FileConn(duplicate(t, adapter))
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			frames := [][]byte{shutdownFrame(sender, 1), shutdownFrame(sender, 2)}
			var wire []byte
			for _, frame := range frames {
				wire = binary.BigEndian.AppendUint32(wire, uint32(len(frame)))
				wire = append(wire, frame...)
			}
			wire = append(wire, test.suffix...)
			if _, err := peer.Write(wire); err != nil {
				t.Fatal(err)
			}
			if err := peer.(*net.UnixConn).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			for _, frame := range frames {
				readShutdownFrame(t, receiver, frame)
			}
			if _, err := io.Copy(io.Discard, peer); err != nil {
				t.Fatalf("adapter did not close after invalid input: %v", err)
			}
			closed := make(chan error, 1)
			go func() {
				closed <- adapter.Close()
			}()
			select {
			case err := <-closed:
				if err == nil {
					t.Fatal("Close lost the stream error")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Close did not finish after invalid input")
			}
		})
	}
}

func newShutdownNetwork(t *testing.T) *vmnet.Network {
	t.Helper()
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, vmnet.WithMTU(1500))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release(config) })
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release((*objc.Pointer)(network)) })
	return network
}

func shutdownFrame(sender *vmnet.Interface, marker byte) []byte {
	frame := bytes.Repeat([]byte{marker}, 60)
	copy(frame, []byte{255, 255, 255, 255, 255, 255})
	copy(frame[6:], sender.MACAddress())
	binary.BigEndian.PutUint16(frame[12:], 0x88b5)
	return frame
}

func readShutdownFrame(t *testing.T, receiver *vmnet.Interface, want []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	buffer := make([]byte, receiver.MaxPacketSize())
	for {
		n, err := receiver.ReadPacket(ctx, buffer)
		if err != nil {
			t.Fatal(err)
		}
		if n < 14 || binary.BigEndian.Uint16(buffer[12:]) != 0x88b5 {
			continue
		}
		if !bytes.Equal(buffer[:n], want) {
			t.Fatalf("received frame = %x, want %x", buffer[:n], want)
		}
		return
	}
}
