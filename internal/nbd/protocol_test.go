package nbd

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

func TestOversizedRequestClosesConnection(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	_, listener := testServer(t, image)
	conn := testClient(t, listener, image.Size(), 512, false)
	header := request{
		Magic:  0x25609513,
		Kind:   1,
		Length: 32<<20 + 1,
	}
	if err := binary.Write(conn, binary.BigEndian, header); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("oversized WRITE did not close the connection: %v", err)
	}
}

func TestInvalidNegotiationClosesConnection(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	_, listener := testServer(t, image)
	for _, name := range []string{"client flags", "option length"} {
		t.Run(name, func(t *testing.T) {
			conn, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			if _, err := io.ReadFull(conn, make([]byte, 18)); err != nil {
				t.Fatal(err)
			}
			if name == "client flags" {
				binary.Write(conn, binary.BigEndian, uint32(7))
			} else {
				binary.Write(conn, binary.BigEndian, uint32(3))
				binary.Write(conn, binary.BigEndian, uint64(0x49484156454f5054))
				binary.Write(conn, binary.BigEndian, uint32(7))
				binary.Write(conn, binary.BigEndian, ^uint32(0))
			}
			if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("invalid negotiation did not close connection: %v", err)
			}
		})
	}
}

func TestBackendErrorsProduceErrorReplies(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	_, listener := testServer(t, image)
	conn := testClient(t, listener, image.Size(), 512, false)
	// Even an image closed against the ownership contract must not produce a
	// successful READ header followed by a truncated payload.
	if err := image.Close(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []uint16{0, 3, 1} {
		length := uint32(512)
		var data []byte
		if kind == 3 {
			length = 0
		} else if kind == 1 {
			data = make([]byte, length)
		}
		if errno, _ := command(t, conn, kind, 0, 0, length, data); errno != 5 {
			t.Fatalf("command %d returned %d, want EIO", kind, errno)
		}
	}
}

func TestExportNamePadding(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	_, listener := testServer(t, image)
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, make([]byte, 18)); err != nil {
		t.Fatal(err)
	}
	binary.Write(conn, binary.BigEndian, uint32(1))
	binary.Write(conn, binary.BigEndian, uint64(0x49484156454f5054))
	binary.Write(conn, binary.BigEndian, uint32(1))
	binary.Write(conn, binary.BigEndian, uint32(0))
	var reply [134]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint64(reply[:]) != uint64(image.Size()) || binary.BigEndian.Uint16(reply[8:]) != 13 {
		t.Fatalf("invalid EXPORT_NAME geometry %x", reply[:10])
	}
	for _, value := range reply[10:] {
		if value != 0 {
			t.Fatal("nonzero EXPORT_NAME padding")
		}
	}
	if errno, _ := command(t, conn, 0, 0, 0, 512, nil); errno != 0 {
		t.Fatalf("EXPORT_NAME read error %d", errno)
	}
}
