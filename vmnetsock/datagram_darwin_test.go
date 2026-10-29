package vmnetsock

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
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
	for i := range frames {
		frames[i] = make([]byte, 1500)
	}
	for _, packets := range [][][]byte{
		{[]byte("abc"), []byte("d"), bytes.Repeat([]byte{42}, 1500)},
		{[]byte("single")},
		{[]byte("reused"), []byte("buffers")},
	} {
		if err := writeDatagramPackets(writer, packets); err != nil {
			t.Fatal(err)
		}
		n, err := readDatagramPackets(reader, frames, 1500)
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
		n, err := readDatagramPackets(reader, frames, 1500)
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
			frames := [][]byte{make([]byte, 3), make([]byte, 3), make([]byte, 3)}
			n, err := readDatagramPackets(reader, frames, 3)
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
		_, err := readDatagramPackets(reader, [][]byte{make([]byte, 1500)}, 1500)
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
	frames := [][]byte{make([]byte, 3), make([]byte, 3), make([]byte, 3)}
	if err := conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if n, err := readDatagramPackets(reader, frames, 3); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("empty socket read = %d, %v", n, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if n, err := readDatagramPackets(reader, frames, 3); n != 1 || err != nil || string(frames[0]) != "abc" {
		t.Fatalf("read after deadline = %d, %q, %v", n, frames[0], err)
	}
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
	writer, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	packets := [][]byte{
		bytes.Repeat([]byte{1}, 1500),
		bytes.Repeat([]byte{2}, 1500),
		bytes.Repeat([]byte{3}, 1500),
	}
	if err := writeDatagramPackets(writer, packets); !errors.Is(err, unix.ENOBUFS) {
		t.Fatalf("partial send = %v, want ENOBUFS", err)
	}
	raw, err := peer.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var received []byte
	var readErr error
	if err := raw.Control(func(fd uintptr) {
		for {
			var n int
			n, readErr = unix.Read(int(fd), frame)
			if readErr != nil {
				return
			}
			if n > 0 && frame[0] != 0 {
				received = append(received, frame[0])
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(readErr, unix.EAGAIN) || !bytes.Equal(received, []byte{1}) {
		t.Fatalf("sent prefix = %v, drain error = %v", received, readErr)
	}
	if err := writeDatagramPackets(writer, packets); err != nil {
		t.Fatalf("send after draining = %v", err)
	}
	for _, want := range packets {
		n, err := peer.Read(frame)
		if err != nil || !bytes.Equal(frame[:n], want) {
			t.Fatalf("recovered send = %x, %v", frame[:n], err)
		}
	}
}
