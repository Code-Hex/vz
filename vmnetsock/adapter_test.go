package vmnetsock_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnetsock"
)

func newInterface(t *testing.T, n *vmnet.Network) *vmnet.Interface {
	t.Helper()
	i, err := vmnet.StartInterfaceWithNetwork(n)
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

func TestAdapterFramesAndLifetime(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	for _, transport := range []vmnetsock.Transport{vmnetsock.Stream, vmnetsock.Datagram} {
		t.Run(string(rune('0'+transport)), func(t *testing.T) {
			c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
			if err != nil {
				t.Fatal(err)
			}
			n, err := vmnet.NewNetwork(c)
			if err != nil {
				t.Fatal(err)
			}
			a, b := newInterface(t, n), newInterface(t, n)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			adapter, err := vmnetsock.New(ctx, a, transport)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			if err := duplicate(t, adapter).Close(); err != nil {
				t.Fatal(err)
			}
			file := duplicate(t, adapter)
			peer, err := net.FileConn(file)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			frame, _ := hex.DecodeString("ffffffffffff02000000000108060001080006040001020000000001c0a8ff02000000000000c0a8ff03000000000000000000000000000000000000")
			if transport == vmnetsock.Stream {
				var header [4]byte
				binary.BigEndian.PutUint32(header[:], uint32(len(frame)))
				_, err = peer.Write(append(header[:], frame...))
			} else {
				_, err = peer.Write(frame)
			}
			if err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, b.MaxPacketSize())
			for {
				count, err := b.ReadPacket(ctx, buffer)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Equal(buffer[:count], frame) {
					break
				}
			}
			if err := b.WritePacket(frame); err != nil {
				t.Fatal(err)
			}
			for {
				var count int
				if transport == vmnetsock.Stream {
					var header [4]byte
					if _, err = io.ReadFull(peer, header[:]); err == nil {
						count = int(binary.BigEndian.Uint32(header[:]))
						if count > len(buffer) {
							t.Fatal("oversized peer frame")
						}
						_, err = io.ReadFull(peer, buffer[:count])
					}
				} else {
					count, err = peer.Read(buffer)
				}
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Equal(buffer[:count], frame) {
					break
				}
			}
			if err := adapter.Close(); err != nil {
				t.Fatal(err)
			}
			if err := adapter.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := peer.Write(frame); err == nil {
				t.Fatal("peer write succeeded after adapter closed")
			}
			if err := a.WritePacket(frame); err != nil {
				t.Fatalf("borrowed interface stopped: %v", err)
			}
		})
	}
}

func TestAdapterInvalidInputs(t *testing.T) {
	for _, i := range []*vmnet.Interface{
		nil,
		{},
	} {
		if _, err := vmnetsock.New(context.Background(), i, vmnetsock.Stream); err == nil {
			t.Fatal("invalid interface accepted")
		}
	}
}

func TestAdapterCancellation(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	c, _ := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	n, err := vmnet.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, transport := range []vmnetsock.Transport{vmnetsock.Stream, vmnetsock.Datagram} {
		i := newInterface(t, n)
		ctx, cancel := context.WithCancel(context.Background())
		a, err := vmnetsock.New(ctx, i, transport)
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
		cancel()
		done := make(chan error, 1)
		go func() {
			done <- a.Close()
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel = %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancel did not stop pumps")
		}
		if _, err := a.Dup(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("adapter file after Close: %v", err)
		}
	}
}

func TestAdapterStreamTermination(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	n, err := vmnet.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		payload   []byte
		wantError bool
	}{
		{
			name:      "peer EOF",
			payload:   nil,
			wantError: false,
		},
		{
			name:      "partial header",
			payload:   []byte{0, 0},
			wantError: true,
		},
		{
			name:      "zero length",
			payload:   []byte{0, 0, 0, 0},
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			i := newInterface(t, n)
			a, err := vmnetsock.New(context.Background(), i, vmnetsock.Stream)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			file := duplicate(t, a)
			connection, err := net.FileConn(file)
			if err != nil {
				t.Fatal(err)
			}
			peer := connection.(*net.UnixConn)
			defer peer.Close()
			if len(test.payload) > 0 {
				if _, err := peer.Write(test.payload); err != nil {
					t.Fatal(err)
				}
			}
			if err := peer.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, peer); err != nil {
				t.Fatalf("peer termination did not close adapter socket: %v", err)
			}
			if _, err := file.Stat(); err != nil {
				t.Fatalf("peer termination closed file before Close: %v", err)
			}
			if err := a.Close(); (err != nil) != test.wantError {
				t.Fatalf("Close = %v", err)
			}
		})
	}
}

func TestAdapterQueuedBurst(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	for _, transport := range []vmnetsock.Transport{vmnetsock.Stream, vmnetsock.Datagram} {
		t.Run(string(rune('0'+transport)), func(t *testing.T) {
			c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { objc.Release(c) })
			network, err := vmnet.NewNetwork(c)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { objc.Release((*objc.Pointer)(network)) })
			sender, receiver := newInterface(t, network), newInterface(t, network)
			frames := make([][]byte, 70)
			for j := range frames {
				frame := bytes.Repeat([]byte{byte(j)}, 60+j)
				copy(frame, []byte{255, 255, 255, 255, 255, 255})
				copy(frame[6:], sender.MACAddress())
				binary.BigEndian.PutUint16(frame[12:], 0x88b5)
				frames[j] = frame
			}
			if n, err := sender.WritePackets(frames); n != len(frames) || err != nil {
				t.Fatalf("burst write = %d, %v", n, err)
			}
			adapter, err := vmnetsock.New(t.Context(), receiver, transport)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			file := duplicate(t, adapter)
			peer, err := net.FileConn(file)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, receiver.MaxPacketSize())
			for received := 0; received < len(frames); {
				var size int
				if transport == vmnetsock.Stream {
					var header [4]byte
					if _, err := io.ReadFull(peer, header[:]); err != nil {
						t.Fatal(err)
					}
					size = int(binary.BigEndian.Uint32(header[:]))
					if size == 0 || size > len(buffer) {
						t.Fatalf("invalid frame size %d", size)
					}
					_, err = io.ReadFull(peer, buffer[:size])
				} else {
					size, err = peer.Read(buffer)
				}
				if err != nil {
					t.Fatalf("frame %d: %v", received, err)
				}
				if size < 14 || binary.BigEndian.Uint16(buffer[12:]) != 0x88b5 {
					continue
				}
				if !bytes.Equal(buffer[:size], frames[received]) {
					t.Fatalf("frame %d differs, length %d", received, size)
				}
				received++
			}
			if err := adapter.Close(); err != nil {
				t.Fatal(err)
			}
			if err := receiver.WritePacket(frames[0]); err != nil {
				t.Fatalf("borrowed interface stopped: %v", err)
			}
		})
	}
}
