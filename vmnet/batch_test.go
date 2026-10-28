package vmnet

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/objc"
)

func batchInterfaces(t *testing.T) (*Interface, *Interface) {
	t.Helper()
	if Available(26) {
		t.Skip("requires macOS 26 SDK")
	}
	c, err := NewNetworkConfiguration(ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release(c) })
	network, err := NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release((*objc.Pointer)(network)) })
	start := func() *Interface {
		i, err := StartInterfaceWithNetwork(network)
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
	return start(), start()
}

func batchFrame(source net.HardwareAddr, index, size int) []byte {
	frame := bytes.Repeat([]byte{byte(index)}, size)
	copy(frame, []byte{255, 255, 255, 255, 255, 255})
	copy(frame[6:], source)
	binary.BigEndian.PutUint16(frame[12:], 0x88b5)
	binary.BigEndian.PutUint32(frame[14:], uint32(index))
	return frame
}

func TestInterfacePacketBatches(t *testing.T) {
	sender, receiver := batchInterfaces(t)
	frames := make([][]byte, 70)
	for j := range frames {
		size := 60
		if j >= 7 {
			size = 1500
		}
		frames[j] = batchFrame(sender.MACAddress(), j, size)
	}
	if n, err := sender.WritePackets(frames); err != nil || n != len(frames) {
		t.Fatalf("write burst = %d, %v", n, err)
	}
	buffers := make([][]byte, 7)
	for j := range buffers {
		buffers[j] = make([]byte, 0, receiver.MaxPacketSize())
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	received := 0
	for received < len(frames) {
		// Notifications coalesce. Already queued packets must not need another wake.
		select {
		case <-receiver.owner.resource.wake:
		default:
		}
		n, err := receiver.ReadPackets(ctx, buffers)
		if err != nil {
			t.Fatalf("read after %d packets: %v", received, err)
		}
		if n < 1 || n > len(buffers) {
			t.Fatalf("read count = %d", n)
		}
		for _, frame := range buffers[:n] {
			if len(frame) < 18 || binary.BigEndian.Uint16(frame[12:]) != 0x88b5 {
				continue
			}
			if received >= len(frames) || !bytes.Equal(frame, frames[received]) {
				t.Fatalf("frame %d differs: length %d", received, len(frame))
			}
			received++
		}
	}
}

func TestInterfaceBatchValidation(t *testing.T) {
	sender, receiver := batchInterfaces(t)
	for _, iface := range []*Interface{nil, {}} {
		if _, err := iface.ReadPackets(t.Context(), nil); err == nil {
			t.Fatal("invalid interface accepted")
		}
		if _, err := iface.WritePackets(nil); err == nil {
			t.Fatal("invalid interface accepted")
		}
	}
	if n, err := sender.WritePackets(nil); n != 0 || err != nil {
		t.Fatalf("empty write = %d, %v", n, err)
	}
	if n, err := receiver.ReadPackets(t.Context(), nil); n != 0 || err != nil {
		t.Fatalf("empty read = %d, %v", n, err)
	}
	frame := batchFrame(sender.MACAddress(), 0, 60)
	for _, invalid := range [][]byte{nil, make([]byte, sender.MaxPacketSize()+1)} {
		frames := make([][]byte, 40)
		for j := range frames {
			frames[j] = frame
		}
		frames[len(frames)-1] = invalid
		if n, err := sender.WritePackets(frames); n != 0 || err == nil {
			t.Fatalf("invalid write = %d, %v", n, err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	buffers := [][]byte{make([]byte, receiver.MaxPacketSize())}
	for {
		n, err := receiver.ReadPackets(ctx, buffers)
		if errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range buffers[:n] {
			if bytes.Equal(got, frame) {
				t.Fatal("invalid batch sent a prefix")
			}
		}
	}
	if err := sender.WritePacket(frame); err != nil {
		t.Fatal(err)
	}
	buffers = append(buffers, make([]byte, receiver.MaxPacketSize()-1))
	if n, err := receiver.ReadPackets(t.Context(), buffers); n != 0 || !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("short batch = %d, %v", n, err)
	}
	if _, err := receiver.ReadPacket(t.Context(), buffers[0][:0]); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("single read lost len contract: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel2()
	for {
		n, err := receiver.ReadPackets(ctx2, buffers[:1])
		if err != nil {
			t.Fatal(err)
		}
		if n == 1 && bytes.Equal(buffers[0], frame) {
			break
		}
	}
}

func TestInterfaceBatchWaitCancellationAndStop(t *testing.T) {
	_, receiver := batchInterfaces(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := make(chan error, 1)
	read := func(ctx context.Context) error {
		buffers := [][]byte{make([]byte, receiver.MaxPacketSize())}
		for {
			if _, err := receiver.ReadPackets(ctx, buffers); err != nil {
				return err
			}
		}
	}
	go func() { first <- read(ctx) }()
	deadline := time.After(3 * time.Second)
	for len(receiver.owner.resource.readers) == 0 {
		select {
		case <-deadline:
			t.Fatal("first reader did not acquire token")
		default:
			runtime.Gosched()
		}
	}
	secondCtx, cancelSecond := context.WithCancel(t.Context())
	second := make(chan error, 1)
	go func() { second <- read(secondCtx) }()
	cancelSecond()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued reader did not cancel")
	}
	go func() { second <- read(t.Context()) }()
	if err := receiver.Stop(); err != nil {
		t.Fatal(err)
	}
	for _, result := range []<-chan error{first, second} {
		select {
		case err := <-result:
			if !errors.Is(err, net.ErrClosed) {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Stop did not interrupt readers")
		}
	}
	if _, err := receiver.WritePackets([][]byte{{1}}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after stop: %v", err)
	}
}
