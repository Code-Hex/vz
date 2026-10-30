package vmnetsock

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestDatagramPackets(t *testing.T) {
	conn, file, err := socketPair(Datagram, 1500)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer file.Close()
	peerConn, err := net.FileConn(file)
	if err != nil {
		t.Fatal(err)
	}
	peer := peerConn.(*net.UnixConn)
	defer peer.Close()
	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	writer, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := peer.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	frames := make([][]byte, 32)
	buffer := make([]byte, 32*1500)
	for _, packets := range [][][]byte{
		{[]byte("abc"), []byte("d"), bytes.Repeat([]byte{42}, 1500)},
		{[]byte("single")},
		{[]byte("reused"), []byte("buffers")},
	} {
		if err := writeDatagramPackets(writer, packets); err != nil {
			t.Fatal(err)
		}
		n, err := readDatagramPackets(reader, frames, buffer, 1500)
		if err != nil || n != len(packets) {
			t.Fatalf("read = %d, %v, want %d packets", n, err, len(packets))
		}
		for i, want := range packets {
			if !bytes.Equal(frames[i], want) {
				t.Fatalf("packet %d = %x, want %x", i, frames[i], want)
			}
		}
	}
	for i := range 35 {
		if _, err := conn.Write([]byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	for received := 0; received < 35; {
		n, err := readDatagramPackets(reader, frames, buffer, 1500)
		if err != nil || n == 0 || n > 32 {
			t.Fatalf("bounded read = %d, %v", n, err)
		}
		for _, frame := range frames[:n] {
			if !bytes.Equal(frame, []byte{byte(received)}) {
				t.Fatalf("packet %d = %x", received, frame)
			}
			received++
		}
	}
}

func TestDatagramPacketsInvalidSuffix(t *testing.T) {
	for _, size := range []int{0, 4, 32} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			conn, file, err := socketPair(Datagram, 3)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			defer file.Close()
			if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			for _, frame := range [][]byte{[]byte("abc"), make([]byte, size), []byte("end")} {
				if _, err := file.Write(frame); err != nil {
					t.Fatal(err)
				}
			}
			reader, err := conn.SyscallConn()
			if err != nil {
				t.Fatal(err)
			}
			frames := make([][]byte, 3)
			buffer := make([]byte, 9)
			n, err := readDatagramPackets(reader, frames, buffer, 3)
			if n != 1 || err == nil || string(frames[0]) != "abc" {
				t.Fatalf("valid prefix = %q, %d, %v", frames[:n], n, err)
			}
		})
	}
}

func TestDatagramPacketsReadClose(t *testing.T) {
	conn, file, err := socketPair(Datagram, 1500)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer file.Close()
	reader, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := readDatagramPackets(reader, make([][]byte, 1), make([]byte, 1500), 1500)
		done <- err
	}()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("read after close = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close did not interrupt batch read")
	}
}

func TestDatagramPacketsReadDeadline(t *testing.T) {
	conn, file, err := socketPair(Datagram, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer file.Close()
	reader, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	frames := make([][]byte, 3)
	buffer := make([]byte, 9)
	if err := conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if n, err := readDatagramPackets(reader, frames, buffer, 3); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("empty socket read = %d, %v", n, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if n, err := readDatagramPackets(reader, frames, buffer, 3); n != 1 || err != nil || string(frames[0]) != "abc" {
		t.Fatalf("read after deadline = %d, %q, %v", n, frames[0], err)
	}
}

type writeWaitNotifier struct {
	syscall.RawConn
	waiting chan struct{}
}

func (r writeWaitNotifier) Write(f func(uintptr) bool) error {
	return r.RawConn.Write(func(fd uintptr) bool {
		ready := f(fd)
		if !ready {
			select {
			case r.waiting <- struct{}{}:
			default:
			}
		}
		return ready
	})
}

func TestDatagramPacketsPartialSend(t *testing.T) {
	conn, file, err := socketPair(Datagram, 1500)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer file.Close()
	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	peerConn, err := net.FileConn(file)
	if err != nil {
		t.Fatal(err)
	}
	defer peerConn.Close()
	peer := peerConn.(*net.UnixConn)
	if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 1500)
	saturateSocket(t, conn, frame)
	if _, err := peer.Read(frame); err != nil {
		t.Fatal(err)
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	writer := writeWaitNotifier{RawConn: raw, waiting: make(chan struct{}, 1)}
	packets := [][]byte{
		bytes.Repeat([]byte{1}, 1500),
		bytes.Repeat([]byte{2}, 1500),
		bytes.Repeat([]byte{3}, 1500),
	}
	done := make(chan error, 1)
	go func() { done <- writeDatagramPackets(writer, packets) }()
	select {
	case <-writer.waiting:
	case err := <-done:
		t.Fatalf("partial send returned before the peer drained: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("partial send did not reach backpressure")
	}

	for {
		n, err := peer.Read(frame)
		if err != nil {
			t.Fatal(err)
		}
		if frame[0] != 0 {
			if !bytes.Equal(frame[:n], packets[0]) {
				t.Fatalf("first packet = %x", frame[:n])
			}
			break
		}
	}
	for _, want := range packets[1:] {
		n, err := peer.Read(frame)
		if err != nil || !bytes.Equal(frame[:n], want) {
			t.Fatalf("resumed send = %x, %v", frame[:n], err)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("send after draining = %v", err)
	}
	rawPeer, err := peer.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var readErr error
	if err := rawPeer.Control(func(fd uintptr) {
		_, readErr = unix.Read(int(fd), frame)
	}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(readErr, unix.EAGAIN) {
		t.Fatalf("extra packet after resumed send: %v", readErr)
	}
}

func TestDatagramPacketsWriteInterrupted(t *testing.T) {
	for _, test := range []struct {
		name      string
		interrupt func(*net.UnixConn) error
		want      error
	}{
		{
			name:      "close",
			interrupt: (*net.UnixConn).Close,
			want:      net.ErrClosed,
		},
		{
			name: "deadline",
			interrupt: func(conn *net.UnixConn) error {
				return conn.SetWriteDeadline(time.Now())
			},
			want: os.ErrDeadlineExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, file, err := socketPair(Datagram, 1500)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			defer file.Close()
			if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			saturateSocket(t, conn, make([]byte, 1500))
			raw, err := conn.SyscallConn()
			if err != nil {
				t.Fatal(err)
			}
			writer := writeWaitNotifier{RawConn: raw, waiting: make(chan struct{}, 1)}
			done := make(chan error, 1)
			go func() { done <- writeDatagramPackets(writer, [][]byte{make([]byte, 1500)}) }()
			select {
			case <-writer.waiting:
			case err := <-done:
				t.Fatalf("send returned before interruption: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("send did not reach backpressure")
			}
			if err := test.interrupt(conn); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, test.want) {
					t.Fatalf("interrupted send = %v, want %v", err, test.want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("interruption did not release the writer")
			}
		})
	}
}

func TestDatagramPacketsMixedSizes(t *testing.T) {
	conn, file, err := socketPair(Datagram, 65550)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer file.Close()
	peer, err := net.FileConn(file)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetWriteDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	frames := make([][]byte, 32)
	buffer := make([]byte, 256000)
	small := make([]int, 32)
	for i := range small {
		small[i] = 1500
	}
	for _, lengths := range [][]int{
		small,
		{60, 65550, 1500, 65550, 61, 65550, 60},
		{1500, 60, 1514, 61},
	} {
		packets := make([][]byte, len(lengths))
		for i, length := range lengths {
			packets[i] = bytes.Repeat([]byte{byte(i + 1)}, length)
			if _, err := peer.Write(packets[i]); err != nil {
				t.Fatal(err)
			}
		}
		for received := 0; received < len(packets); {
			n, err := readDatagramPackets(raw, frames, buffer, 65550)
			if err != nil || n == 0 || n > len(packets)-received {
				t.Fatalf("read = %d, %v, with %d packets remaining", n, err, len(packets)-received)
			}
			if len(packets) == 32 && received == 0 && n != 32 {
				t.Fatalf("small packet batch = %d, want 32 packets", n)
			}
			for _, frame := range frames[:n] {
				if !bytes.Equal(frame, packets[received]) {
					t.Fatalf("packet %d differs", received)
				}
				received++
			}
		}
	}
}
