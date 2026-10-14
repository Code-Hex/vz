package fileadapter_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnet/fileadapter"
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
	for _, transport := range []fileadapter.Transport{fileadapter.Stream, fileadapter.Datagram} {
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
			adapter, file, err := fileadapter.New(ctx, a, transport)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			defer file.Close()
			peer, err := net.FileConn(file)
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			frame, _ := hex.DecodeString("ffffffffffff02000000000108060001080006040001020000000001c0a8ff02000000000000c0a8ff03000000000000000000000000000000000000")
			if transport == fileadapter.Stream {
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
				if transport == fileadapter.Stream {
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
			if err := adapter.Wait(); err != nil {
				t.Fatal(err)
			}
			if err := adapter.Close(); err != nil {
				t.Fatal(err)
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
		if _, _, err := fileadapter.New(context.Background(), i, fileadapter.Stream); err == nil {
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
	for _, transport := range []fileadapter.Transport{fileadapter.Stream, fileadapter.Datagram} {
		i := newInterface(t, n)
		ctx, cancel := context.WithCancel(context.Background())
		a, f, err := fileadapter.New(ctx, i, transport)
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		done := make(chan error, 1)
		go func() {
			done <- a.Wait()
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel = %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancel did not stop pumps")
		}
		f.Close()
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
			a, f, err := fileadapter.New(context.Background(), i, fileadapter.Stream)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if len(test.payload) > 0 {
				if _, err := f.Write(test.payload); err != nil {
					t.Fatal(err)
				}
			}
			f.Close()
			done := make(chan error, 1)
			go func() {
				done <- a.Wait()
			}()
			select {
			case err := <-done:
				if (err != nil) != test.wantError {
					t.Fatalf("Wait = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("peer termination did not stop adapter")
			}
		})
	}
}
