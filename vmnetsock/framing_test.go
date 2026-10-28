package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
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
		reader := newStreamPacketReader(a, 3)
		var got []string
		for {
			frames, err := reader.readPackets()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, frame := range frames {
				got = append(got, string(frame))
			}
		}
		if len(got) != 2 || got[0] != "abc" || got[1] != "de" {
			t.Fatalf("frames = %q", got)
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
			_, err := newStreamPacketReader(bytes.NewReader(test.data), 3).readPackets()
			if !errors.Is(err, test.want) {
				t.Fatalf("read = %v", err)
			}
		})
	}
	for _, length := range []uint32{0, 4, ^uint32(0)} {
		t.Run("invalid length", func(t *testing.T) {
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], length)
			if _, err := newStreamPacketReader(bytes.NewReader(header[:]), 3).readPackets(); err == nil {
				t.Fatal("invalid length accepted")
			}
		})
	}
	writer := &bytes.Buffer{}
	if err := writeStreamFrames(writer, [][]byte{[]byte("abc")}, make([][4]byte, 1), make(net.Buffers, 2)); err != nil {
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
			var raw syscall.RawConn
			if transport == Datagram {
				raw, err = conn.SyscallConn()
				if err != nil {
					t.Fatal(err)
				}
			}

			done := make(chan error, 1)
			go func() {
				if transport == Stream {
					done <- writeStreamFrames(conn, [][]byte{frame}, make([][4]byte, 1), make(net.Buffers, 2))
				} else {
					done <- writeDatagramPackets(raw, [][]byte{frame})
				}
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

func TestStreamBatchFrames(t *testing.T) {
	conn, file, err := socketPair(Stream, 1500)
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
	if err := conn.SetWriteDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	frames := [][]byte{[]byte("abc"), []byte("d"), bytes.Repeat([]byte{42}, 1500)}
	headers := make([][4]byte, len(frames))
	vectors := make(net.Buffers, 2*len(frames))
	var want bytes.Buffer
	for _, frame := range frames {
		binary.Write(&want, binary.BigEndian, uint32(len(frame)))
		want.Write(frame)
	}
	for range 2 {
		if err := writeStreamFrames(conn, frames, headers, vectors); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, want.Len())
		if _, err := io.ReadFull(peer, got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want.Bytes()) {
			t.Fatal("stream batch changed framing or order")
		}
	}
	writer := &shortWriter{}
	if err := writeStreamFrames(writer, frames, headers, vectors); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short writer = %v", err)
	}
}

func TestStreamPacketReaderSuffix(t *testing.T) {
	for _, test := range []struct {
		name   string
		suffix []byte
		want   error
	}{
		{
			name: "EOF",
			want: io.EOF,
		},
		{
			name:   "partial header",
			suffix: []byte{0, 0},
			want:   io.ErrUnexpectedEOF,
		},
		{
			name:   "partial payload",
			suffix: []byte{0, 0, 0, 3, 'x'},
			want:   io.ErrUnexpectedEOF,
		},
		{
			name:   "invalid length",
			suffix: []byte{0, 0, 0, 4},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte{0, 0, 0, 3, 'a', 'b', 'c', 0, 0, 0, 1, 'd'}, test.suffix...)
			reader := newStreamPacketReader(bytes.NewReader(data), 3)
			frames, err := reader.readPackets()
			if err != nil || len(frames) != 2 {
				t.Fatalf("complete prefix = %q, %v", frames, err)
			}
			if string(frames[0]) != "abc" || string(frames[1]) != "d" {
				t.Fatalf("frames = %q", frames)
			}
			for _, frame := range frames {
				if cap(frame) != len(frame) {
					t.Fatalf("frame capacity = %d, length = %d", cap(frame), len(frame))
				}
			}
			_, err = reader.readPackets()
			if err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("suffix error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestStreamPacketReaderDoesNotWaitForNextFrame(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if err := a.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	continueWrite := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		if _, err := b.Write([]byte{0, 0, 0, 1, 'a', 0, 0, 0, 2, 'b'}); err != nil {
			done <- err
			return
		}
		<-continueWrite
		_, err := b.Write([]byte{'c'})
		done <- err
	}()
	reader := newStreamPacketReader(a, 3)
	frames, err := reader.readPackets()
	close(continueWrite)
	if err != nil || len(frames) != 1 || string(frames[0]) != "a" {
		t.Fatalf("first batch = %q, %v", frames, err)
	}
	frames, err = reader.readPackets()
	if err != nil || len(frames) != 1 || string(frames[0]) != "bc" {
		t.Fatalf("second batch = %q, %v", frames, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestStreamPacketReaderBatchLimits(t *testing.T) {
	for _, size := range []int{1, 1500, 65550, 256000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var data bytes.Buffer
			for i := range 35 {
				binary.Write(&data, binary.BigEndian, uint32(size))
				data.Write(bytes.Repeat([]byte{byte(i)}, size))
			}
			reader := newStreamPacketReader(&data, size)
			got := 0
			for {
				frames, err := reader.readPackets()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(frames) == 0 || len(frames) > 32 || len(frames)*size > 256000 {
					t.Fatalf("batch has %d packets of %d bytes", len(frames), size)
				}
				for _, frame := range frames {
					if len(frame) != size || !bytes.Equal(frame, bytes.Repeat([]byte{byte(got)}, size)) {
						t.Fatalf("packet %d was changed", got)
					}
					got++
				}
			}
			if got != 35 {
				t.Fatalf("received %d packets, want 35", got)
			}
		})
	}
}
