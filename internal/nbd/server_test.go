package nbd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

func testImage(t *testing.T, block diskimagekit.BlockSize) *diskimagekit.DiskImage {
	t.Helper()
	image, err := diskimagekit.CreateASIF(filepath.Join(t.TempDir(), "disk.asif"), 2048, diskimagekit.WithBlockSize(block))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { image.Close() })
	return image
}

func testServer(t *testing.T, image *diskimagekit.DiskImage, options ...Option) (*Server, net.Listener) {
	t.Helper()
	server, err := NewServer(image, options...)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
		if err := <-served; !errors.Is(err, net.ErrClosed) {
			t.Errorf("Serve returned %v, want net.ErrClosed", err)
		}
	})
	return server, listener
}

func testClient(t *testing.T, listener net.Listener, size int64, block uint32, readOnly bool) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	var greeting [18]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		t.Fatal(err)
	}
	if string(greeting[:8]) != "NBDMAGIC" || string(greeting[8:16]) != "IHAVEOPT" || binary.BigEndian.Uint16(greeting[16:]) != 3 {
		t.Fatalf("bad greeting %x", greeting)
	}
	binary.Write(conn, binary.BigEndian, uint32(3))
	// GO for the default export, requesting block-size information.
	request := []byte{0, 0, 0, 0, 0, 1, 0, 3}
	binary.Write(conn, binary.BigEndian, uint64(0x49484156454f5054))
	binary.Write(conn, binary.BigEndian, uint32(7))
	binary.Write(conn, binary.BigEndian, uint32(len(request)))
	conn.Write(request)
	seenSize, seenBlock := false, false
	for {
		var header struct {
			Magic  uint64
			Option uint32
			Type   uint32
			Length uint32
		}
		if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
			t.Fatal(err)
		}
		if header.Magic != 0x3e889045565a9 || header.Option != 7 || header.Length > 1024 {
			t.Fatalf("invalid option response %+v", header)
		}
		data := make([]byte, header.Length)
		if _, err := io.ReadFull(conn, data); err != nil {
			t.Fatal(err)
		}
		if header.Type == 1 {
			break
		}
		if header.Type != 3 || len(data) < 2 {
			t.Fatalf("unexpected option response %+v %x", header, data)
		}
		switch binary.BigEndian.Uint16(data) {
		case 0:
			flags := uint16(1 | 4 | 8)
			if readOnly {
				flags |= 2
			}
			if len(data) != 12 || int64(binary.BigEndian.Uint64(data[2:])) != size || binary.BigEndian.Uint16(data[10:]) != flags {
				t.Fatalf("bad export info %x", data)
			}
			seenSize = true
		case 3:
			if len(data) != 14 || binary.BigEndian.Uint32(data[2:]) != block || binary.BigEndian.Uint32(data[6:]) != block || binary.BigEndian.Uint32(data[10:]) != 32<<20 {
				t.Fatalf("bad block info %x", data)
			}
			seenBlock = true
		}
	}
	if !seenSize || !seenBlock {
		t.Fatal("missing export geometry")
	}
	return conn
}

func command(t *testing.T, conn net.Conn, kind, flags uint16, offset uint64, length uint32, data []byte) (uint32, []byte) {
	t.Helper()
	header := struct {
		Magic  uint32
		Flags  uint16
		Type   uint16
		Handle uint64
		Offset uint64
		Length uint32
	}{
		Magic:  0x25609513,
		Flags:  flags,
		Type:   kind,
		Handle: 0x1020304050607080,
		Offset: offset,
		Length: length,
	}
	if err := binary.Write(conn, binary.BigEndian, header); err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		if _, err := conn.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	var reply struct {
		Magic  uint32
		Error  uint32
		Handle uint64
	}
	if err := binary.Read(conn, binary.BigEndian, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Magic != 0x67446698 || reply.Handle != header.Handle {
		t.Fatalf("bad transmission reply %+v", reply)
	}
	var result []byte
	if kind == 0 && reply.Error == 0 {
		result = make([]byte, length)
		if _, err := io.ReadFull(conn, result); err != nil {
			t.Fatal(err)
		}
	}
	return reply.Error, result
}

func TestReadWriteFlush(t *testing.T) {
	for _, block := range []diskimagekit.BlockSize{diskimagekit.BlockSize512, diskimagekit.BlockSize4096} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			image := testImage(t, block)
			_, listener := testServer(t, image)
			conn := testClient(t, listener, image.Size(), uint32(block), false)
			data := bytes.Repeat([]byte("A"), int(block))
			for _, flags := range []uint16{0, 1} {
				if errno, _ := command(t, conn, 1, flags, uint64(block), uint32(block), data); errno != 0 {
					t.Fatalf("write error %d", errno)
				}
			}
			if errno, _ := command(t, conn, 3, 1, 0, 0, nil); errno != 0 {
				t.Fatalf("flush error %d", errno)
			}
			if errno, got := command(t, conn, 0, 1, uint64(block), uint32(block), nil); errno != 0 || !bytes.Equal(got, data) {
				t.Fatalf("read error %d or wrong data", errno)
			}
			if errno, _ := command(t, conn, 4, 0, uint64(block), uint32(block), nil); errno != 22 {
				t.Fatalf("unadvertised trim error %d, want EINVAL", errno)
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			conn = testClient(t, listener, image.Size(), uint32(block), false)
			if errno, got := command(t, conn, 0, 0, uint64(block), uint32(block), nil); errno != 0 || !bytes.Equal(got, data) {
				t.Fatal("reconnected client could not read the earlier write")
			}
		})
	}
}

func TestReadOnlyAndInvalidRequests(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	_, listener := testServer(t, image, WithReadOnly())
	conn := testClient(t, listener, image.Size(), 512, true)
	if errno, _ := command(t, conn, 1, 0, 0, 512, bytes.Repeat([]byte("X"), 512)); errno != 1 {
		t.Fatalf("read-only write error %d", errno)
	}
	for _, offset := range []uint64{1, uint64(image.Size()), ^uint64(0) - 511} {
		if errno, _ := command(t, conn, 0, 0, offset, 512, nil); errno != 22 {
			t.Fatalf("invalid offset %d error %d", offset, errno)
		}
	}
	if errno, got := command(t, conn, 0, 0, 0, 512, nil); errno != 0 || !bytes.Equal(got, make([]byte, 512)) {
		t.Fatal("rejected write changed the image or desynchronized the connection")
	}
}

func TestCloseStopsPendingConnections(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, listener := testServer(t, image)
	idle, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	conn := testClient(t, listener, image.Size(), 512, false)
	closed := make(chan error, 2)
	go func() { closed <- server.Close() }()
	go func() { closed <- server.Close() }()
	for range 2 {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not stop an idle connection")
		}
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection remained open")
	}
	if _, err := image.ReadAt(make([]byte, 512), 0); err != nil {
		t.Fatalf("Close closed the borrowed image: %v", err)
	}
}

func TestReadOnlyImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.raw")
	image, err := diskimagekit.CreateRAW(path, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := image.Close(); err != nil {
		t.Fatal(err)
	}
	image, err = diskimagekit.Open(path, diskimagekit.WithOpenMode(diskimagekit.ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { image.Close() })
	_, listener := testServer(t, image)
	conn := testClient(t, listener, image.Size(), 512, true)
	if errno, _ := command(t, conn, 1, 1, 0, 512, make([]byte, 512)); errno != 1 {
		t.Fatalf("read-only image write error %d", errno)
	}
}
