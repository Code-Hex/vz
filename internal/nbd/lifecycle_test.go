package nbd

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

func TestCloseBeforeServe(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, err := NewServer(image)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := server.Serve(listener); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Serve after Close: %v", err)
	}
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Serve took ownership of rejected listener: %v", err)
	}
	conn.Close()
}

func TestServeOnce(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, listener := testServer(t, image)
	testClient(t, listener, image.Size(), 512, false)
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := server.Serve(other); err == nil {
		t.Fatal("second Serve succeeded")
	}
	conn, err := net.Dial("tcp", other.Addr().String())
	if err != nil {
		t.Fatalf("second listener was closed: %v", err)
	}
	conn.Close()
}

type delayedListener struct {
	net.Listener
	accepted chan struct{}
	release  chan struct{}
	closed   chan struct{}
	once     sync.Once
}

func (l *delayedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		close(l.accepted)
		<-l.release
	}
	return conn, err
}

func (l *delayedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

func TestCloseAfterAccept(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, err := NewServer(image)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	delayed := &delayedListener{
		Listener: listener,
		accepted: make(chan struct{}),
		release:  make(chan struct{}),
		closed:   make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(delayed.release) })
	defer func() {
		release()
		server.Close()
	}()
	served := make(chan error, 1)
	go func() { served <- server.Serve(delayed) }()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case <-delayed.accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not accept the client")
	}
	closed := make(chan error, 1)
	go func() { closed <- server.Close() }()
	select {
	case <-delayed.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not close the listener")
	}
	release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked after Accept returned a connection")
	}
	select {
	case err := <-served:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("accepted connection was not closed: %v", err)
	}
}

func TestCloseDuringWritePayload(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, listener := testServer(t, image)
	conn := testClient(t, listener, image.Size(), 512, false)
	header := request{
		Magic:  requestMagic,
		Kind:   commandWrite,
		Length: 512,
	}
	if err := binary.Write(conn, binary.BigEndian, header); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- server.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not interrupt a partial WRITE payload")
	}
}

type singleConnectionListener struct {
	net.Listener
	conn net.Conn
}

func (l *singleConnectionListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		conn := l.conn
		l.conn = nil
		return conn, nil
	}
	return l.Listener.Accept()
}

func TestCloseDuringUnreadReply(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, err := NewServer(image)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted, client := net.Pipe()
	defer func() {
		client.Close()
		listener.Close()
		server.Close()
	}()
	client.SetDeadline(time.Now().Add(5 * time.Second))
	served := make(chan error, 1)
	go func() {
		served <- server.Serve(&singleConnectionListener{
			Listener: listener,
			conn:     accepted,
		})
	}()
	if _, err := io.ReadFull(client, make([]byte, 18)); err != nil {
		t.Fatal(err)
	}
	var handshake [20]byte
	binary.BigEndian.PutUint32(handshake[:], 3)
	binary.BigEndian.PutUint64(handshake[4:], 0x49484156454f5054)
	binary.BigEndian.PutUint32(handshake[12:], 1)
	if _, err := client.Write(handshake[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(client, make([]byte, 10)); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(client, binary.BigEndian, request{
		Magic:  requestMagic,
		Kind:   commandRead,
		Length: 512,
	}); err != nil {
		t.Fatal(err)
	}
	// Reading only the header leaves the payload blocked on the unbuffered pipe.
	var reply [16]byte
	if _, err := io.ReadFull(client, reply[:]); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(reply[:]) != 0x67446698 || binary.BigEndian.Uint32(reply[4:]) != 0 {
		t.Fatalf("READ did not return a success header: %x", reply)
	}
	closed := make(chan error, 1)
	go func() { closed <- server.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt an unread reply")
	}
	if err := <-served; !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Serve returned %v, want net.ErrClosed", err)
	}
}

func TestListenerCloseStopsClients(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	server, listener := testServer(t, image)
	conn := testClient(t, listener, image.Size(), 512, false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("client connection survived the accept failure: %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientDisconnectDoesNotStopExport(t *testing.T) {
	image := testImage(t, diskimagekit.BlockSize512)
	_, listener := testServer(t, image)
	conn := testClient(t, listener, image.Size(), 512, false)
	header := request{
		Magic: 0x25609513,
		Kind:  2,
	}
	if err := binary.Write(conn, binary.BigEndian, header); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("DISCONNECT did not close the connection: %v", err)
	}
	reconnected := testClient(t, listener, image.Size(), 512, false)
	if errno, _ := command(t, reconnected, 0, 0, 0, 512, nil); errno != 0 {
		t.Fatalf("read after reconnect returned %d", errno)
	}
}
