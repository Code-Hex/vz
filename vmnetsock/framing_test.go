package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStreamFrames(t *testing.T) {
	t.Run("split and concatenated", func(t *testing.T) {
		a, b := net.Pipe()
		defer a.Close()
		defer b.Close()
		go func() {
			defer b.Close()
			for _, part := range [][]byte{
				{0},
				{0, 0},
				{3, 'a'},
				{'b'},
				{'c', 0, 0, 0, 2, 'd', 'e'},
			} {
				_, _ = b.Write(part)
			}
		}()
		buf := make([]byte, 3)
		for _, want := range []string{"abc", "de"} {
			n, err := readStreamFrame(a, buf)
			if err != nil || string(buf[:n]) != want {
				t.Fatalf("frame %q, %v, want %q", buf[:n], err, want)
			}
		}
		if _, err := readStreamFrame(a, buf); !errors.Is(err, io.EOF) {
			t.Fatalf("EOF = %v", err)
		}
	})
	for _, test := range []struct {
		name string
		data []byte
		want error
	}{
		{
			name: "empty header",
			data: nil,
			want: io.EOF,
		},
		{
			name: "partial header",
			data: []byte{0, 0},
			want: io.ErrUnexpectedEOF,
		},
		{
			name: "partial payload",
			data: []byte{0, 0, 0, 3, 'a'},
			want: io.ErrUnexpectedEOF,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := readStreamFrame(bytes.NewReader(test.data), make([]byte, 3))
			if !errors.Is(err, test.want) {
				t.Fatalf("read = %v", err)
			}
		})
	}
	for _, length := range []uint32{0, 4, ^uint32(0)} {
		t.Run("invalid length", func(t *testing.T) {
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], length)
			if _, err := readStreamFrame(bytes.NewReader(header[:]), make([]byte, 3)); err == nil {
				t.Fatal("invalid length accepted")
			}
		})
	}
	writer := &shortWriter{}
	if err := writeStreamFrame(writer, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(writer.Bytes(), []byte{0, 0, 0, 3, 'a', 'b', 'c'}) {
		t.Fatalf("encoded = %x", writer.Bytes())
	}
}

type shortWriter struct {
	bytes.Buffer
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return w.Buffer.Write(p)
}

func TestDatagramFrames(t *testing.T) {
	conn, file, err := socketPair(Datagram, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer file.Close()
	if _, err := file.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	n, err := readDatagramFrame(conn, buf)
	if err != nil || string(buf[:n]) != "abc" {
		t.Fatalf("datagram = %q, %v", buf[:n], err)
	}
	if _, err := file.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if _, err := readDatagramFrame(conn, buf); err == nil {
		t.Fatal("truncated datagram accepted")
	}
	if _, err := file.Write(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := readDatagramFrame(conn, buf); err == nil {
		t.Fatal("empty datagram accepted")
	}
}

func TestSocketBackpressureClose(t *testing.T) {
	for _, transport := range []Transport{Stream, Datagram} {
		t.Run(fmt.Sprint(transport), func(t *testing.T) {
			conn, file, err := socketPair(transport, 65550)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			frame := make([]byte, 65550)
			saturateSocket(t, conn, frame)

			done := make(chan error, 1)
			go func() {
				_, err := conn.Write(frame)
				done <- err
			}()
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, net.ErrClosed) && !(transport == Datagram && errors.Is(err, unix.ENOBUFS)) {
					t.Fatalf("blocked write = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("close did not interrupt socket write")
			}
		})
	}
}

func saturateSocket(t *testing.T, conn *net.UnixConn, frame []byte) {
	t.Helper()
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}

	var fillErr error
	if err := raw.Control(func(fd uintptr) {
		for attempt := 0; attempt < 1024; attempt++ {
			_, fillErr = unix.Write(int(fd), frame)
			if fillErr != nil {
				return
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(fillErr, unix.EAGAIN) && !errors.Is(fillErr, unix.ENOBUFS) {
		t.Fatalf("socket did not reach backpressure: %v", fillErr)
	}
}
