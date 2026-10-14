package fileadapter

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestAdapterBackpressureCleanup(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	for _, transport := range []Transport{Stream, Datagram} {
		for _, cancelContext := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/cancel=%v", transport, cancelContext), func(t *testing.T) {
				c, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
				if err != nil {
					t.Fatal(err)
				}
				n, err := vmnet.NewNetwork(c)
				if err != nil {
					t.Fatal(err)
				}
				receiver, err := vmnet.StartInterfaceWithNetwork(n)
				if err != nil {
					t.Fatal(err)
				}
				defer receiver.Stop()
				sender, err := vmnet.StartInterfaceWithNetwork(n)
				if err != nil {
					t.Fatal(err)
				}
				defer sender.Stop()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				a, file, err := New(ctx, receiver, transport)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				defer file.Close()

				saturateSocket(t, a.state.conn, make([]byte, receiver.MaxPacketSize()))

				frame, _ := hex.DecodeString("ffffffffffff02000000000108060001080006040001020000000001c0a8ff02000000000000c0a8ff03000000000000000000000000000000000000")
				sendPacketBurst(t, sender, frame)
				if transport == Datagram {
					peer, err := net.FileConn(file)
					if err != nil {
						t.Fatal(err)
					}
					defer peer.Close()

					checkDatagramRecovery(t, a, sender, peer, frame, receiver.MaxPacketSize())
				}

				done := make(chan error, 1)
				if cancelContext {
					cancel()
					go func() {
						done <- a.Wait()
					}()
				} else {
					go func() {
						done <- a.Close()
					}()
				}
				select {
				case err := <-done:
					if cancelContext {
						if !errors.Is(err, context.Canceled) {
							t.Fatalf("cancel = %v", err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("adapter did not join pumps under backpressure")
				}

				check, cancelCheck := context.WithCancel(context.Background())
				cancelCheck()
				if _, err := receiver.ReadPacket(check, make([]byte, receiver.MaxPacketSize())); errors.Is(err, net.ErrClosed) {
					t.Fatal("adapter stopped borrowed interface")
				}
			})
		}
	}
}

func sendPacketBurst(t *testing.T, sender *vmnet.Interface, frame []byte) {
	t.Helper()

	for range 1024 {
		if err := sender.WritePacket(frame); err != nil {
			var nativeErr *vmnet.Error
			if !errors.As(err, &nativeErr) || nativeErr.Code != 1007 {
				t.Fatal(err)
			}
			break
		}
	}
}

func checkDatagramRecovery(t *testing.T, a *Adapter, sender *vmnet.Interface, peer net.Conn, frame []byte, maxSize int) {
	t.Helper()

	if err := peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, maxSize)
	for {
		_, err := peer.Read(buffer)
		if err != nil {
			var timeout net.Error
			if !errors.As(err, &timeout) || !timeout.Timeout() {
				t.Fatal(err)
			}
			break
		}
	}

	select {
	case <-a.state.done:
		t.Fatalf("saturation stopped adapter: %v", a.Wait())
	default:
	}

	sentinel := bytes.Clone(frame)
	sentinel[len(sentinel)-1] = 0x7f
	if err := sender.WritePacket(sentinel); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		count, err := peer.Read(buffer)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(buffer[:count], sentinel) {
			break
		}
	}
}
