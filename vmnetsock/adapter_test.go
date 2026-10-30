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
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnetsock"
)

func newInterface(t *testing.T, n *vmnet.Network, opts ...vmnet.InterfaceOption) *vmnet.Interface {
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

func TestAdapterFramesAndLifetime(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	for _, test := range []struct {
		name      string
		transport vmnetsock.Transport
		convert   bool
		tso       bool
	}{
		{name: "stream", transport: vmnetsock.Stream},
		{name: "datagram", transport: vmnetsock.Datagram},
		{name: "stream Virtio conversion", transport: vmnetsock.Stream, convert: true},
		{name: "datagram Virtio conversion", transport: vmnetsock.Datagram, convert: true},
		{name: "stream TSO conversion", transport: vmnetsock.Stream, convert: true, tso: true},
		{name: "datagram TSO conversion", transport: vmnetsock.Datagram, convert: true, tso: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := test.transport
			c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, vmnet.WithMTU(1500))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { objc.Release(c) })
			n, err := vmnet.NewNetwork(c)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { objc.Release((*objc.Pointer)(n)) })
			a := newInterface(t, n, vmnet.WithInterfaceVirtioHeader(test.convert), vmnet.WithInterfaceTSO(test.tso))
			b := newInterface(t, n)
			if a.VirtioHeaderEnabled() != test.convert || a.TSOEnabled() != test.tso {
				t.Fatal("unexpected interface offload configuration")
			}
			var opts []vmnetsock.Option
			if test.convert {
				opts = append(opts, vmnetsock.WithVirtioHeaderConversion())
			} else if adapter, err := vmnetsock.New(context.Background(), a, transport, vmnetsock.WithVirtioHeaderConversion()); err == nil {
				adapter.Close()
				t.Fatal("conversion accepted an interface without Virtio headers")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			adapter, err := vmnetsock.New(ctx, a, transport, opts...)
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
			buffer := make([]byte, b.MaxPacketSize())
			for _, lengths := range [][]int{{60, 128, 61}, {61, 60, 128}} {
				frames := make([][]byte, len(lengths))
				var wire bytes.Buffer
				for i, length := range lengths {
					frames[i] = append(bytes.Clone(frame), bytes.Repeat([]byte{0xa5}, length-len(frame))...)
					if transport == vmnetsock.Stream {
						binary.Write(&wire, binary.BigEndian, uint32(len(frames[i])))
						wire.Write(frames[i])
					} else if _, err := peer.Write(frames[i]); err != nil {
						t.Fatal(err)
					}
				}
				if transport == vmnetsock.Stream {
					if _, err := peer.Write(wire.Bytes()); err != nil {
						t.Fatal(err)
					}
				}
				for _, want := range frames {
					for {
						count, err := b.ReadPacket(ctx, buffer)
						if err != nil {
							t.Fatal(err)
						}
						if bytes.Equal(buffer[:count], want) {
							break
						}
					}
				}
				for _, packet := range frames {
					if err := b.WritePacket(packet); err != nil {
						t.Fatal(err)
					}
				}
				for _, want := range frames {
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
						if bytes.Equal(buffer[:count], want) {
							break
						}
					}
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
			if test.convert {
				frame = append(make([]byte, 12), frame...)
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
	c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release(c) })
	n, err := vmnet.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release((*objc.Pointer)(n)) })
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
	t.Cleanup(func() { objc.Release(c) })
	n, err := vmnet.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release((*objc.Pointer)(n)) })
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
			c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, vmnet.WithMTU(1500))
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
				size := 60 + j
				if transport == vmnetsock.Datagram {
					size = 1500
				}
				frame := bytes.Repeat([]byte{byte(j)}, size)
				copy(frame, []byte{255, 255, 255, 255, 255, 255})
				copy(frame[6:], sender.MACAddress())
				binary.BigEndian.PutUint16(frame[12:], 0x88b5)
				frames[j] = frame
			}
			if transport == vmnetsock.Stream {
				if n, err := sender.WritePackets(frames); n != len(frames) || err != nil {
					t.Fatalf("burst write = %d, %v", n, err)
				}
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
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if transport == vmnetsock.Datagram {
				if err := peer.(*net.UnixConn).SetReadBuffer(8192); err != nil {
					t.Fatal(err)
				}
				if n, err := sender.WritePackets(frames); n != len(frames) || err != nil {
					t.Fatalf("burst write = %d, %v", n, err)
				}
			}
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
			if transport == vmnetsock.Datagram {
				sentinel := bytes.Clone(frames[0])
				sentinel[len(sentinel)-1] = 0xff
				if err := sender.WritePacket(sentinel); err != nil {
					t.Fatal(err)
				}
				for {
					size, err := peer.Read(buffer)
					if err != nil {
						t.Fatalf("frame after draining burst: %v", err)
					}
					if size < 14 || binary.BigEndian.Uint16(buffer[12:]) != 0x88b5 {
						continue
					}
					if !bytes.Equal(buffer[:size], sentinel) {
						t.Fatal("frame after draining burst differs")
					}
					break
				}
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

func TestAdapterVirtioConversionTSOUnknownConnection(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, vmnet.WithMTU(1500))
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(c)
	n, err := vmnet.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release((*objc.Pointer)(n))
	iface := newInterface(t, n, vmnet.WithInterfaceVirtioHeader(true), vmnet.WithInterfaceTSO(true))
	if !iface.VirtioHeaderEnabled() || !iface.TSOEnabled() {
		t.Fatal("interface did not retain offload configuration")
	}
	adapter, err := vmnetsock.New(context.Background(), iface, vmnetsock.Stream, vmnetsock.WithVirtioHeaderConversion())
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
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	sender := newInterface(t, n)
	// This valid TCP data frame deliberately has no preceding handshake.
	frame, _ := hex.DecodeString("000000000000000000000000080045000029000100004006f6cac0000202c000020104d2005000000001000000015018ffffaea3000078")
	copy(frame, iface.MACAddress())
	copy(frame[6:], sender.MACAddress())
	if err := sender.WritePacket(frame); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 2048)
	for {
		_, err = peer.Read(buffer)
		if err != nil {
			break
		}
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("peer read = %v, want EOF", err)
	}
	if err := adapter.Close(); err == nil || !strings.Contains(err.Error(), "no observed MSS handshake") {
		t.Fatalf("Close = %v, want missing handshake error", err)
	}
}

func TestAdapterTSOCloseDuringTraffic(t *testing.T) {
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
	iface := newInterface(t, network, vmnet.WithInterfaceVirtioHeader(true), vmnet.WithInterfaceTSO(true))
	remote := newInterface(t, network)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	adapter, err := vmnetsock.New(ctx, iface, vmnetsock.Stream, vmnetsock.WithVirtioHeaderConversion())
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
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// Repeat one SYN so both forwarding and MSS observation stay active.
	syn, _ := hex.DecodeString("00000000000000000000000008004500002c000100004006f6c7c0000202c000020104d2005000000001000000006002ffff13b4000002040100")
	copy(syn, remote.MACAddress())
	copy(syn[6:], iface.MACAddress())
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(syn)))
	wire := append(header[:], syn...)
	writesDone := make(chan struct{})
	go func() {
		defer close(writesDone)
		for {
			if _, err := peer.Write(wire); err != nil {
				return
			}
		}
	}()
	buffer := make([]byte, remote.MaxPacketSize())
	for {
		n, err := remote.ReadPacket(ctx, buffer)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(buffer[:n], syn) {
			break
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- adapter.Close() }()
	cancel()
	select {
	case err := <-closed:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not stop forwarding")
	}
	select {
	case <-writesDone:
	case <-time.After(3 * time.Second):
		t.Fatal("peer writes did not stop")
	}
	if _, err := adapter.Dup(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Dup after Close = %v", err)
	}
	if err := iface.WritePacket(append(make([]byte, 12), syn...)); err != nil {
		t.Fatalf("borrowed interface stopped: %v", err)
	}
}
