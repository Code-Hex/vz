package vz_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func TestVirtioSocketListener(t *testing.T) {
	container := newVirtualizationMachine(t)
	socketDevice := container.SocketDevices()[0]
	port := 43218
	wantData := "hello\n"

	listener, err := socketDevice.Listen(uint32(port))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	const readTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), readTimeout)
	defer cancel()
	closed := make(chan struct{})
	stopClose := context.AfterFunc(ctx, func() {
		listener.Close()
		close(closed)
	})
	defer func() {
		if stopClose() {
			close(closed)
		}
		<-closed
	}()

	session := container.NewSession(t)
	defer session.Close()
	var stderr bytes.Buffer
	session.Stderr = &stderr
	cmd := fmt.Sprintf("{ printf he; sleep 0.2; printf 'llo\\n'; } | socat - VSOCK-CONNECT:2:%d", port)
	if err := session.Start(cmd); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := conn.(*vz.VirtioSocketConnection).DestinationPort(); got != uint32(port) {
		t.Fatalf("destination port = %d, want %d", got, port)
	}
	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(wantData))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if got := string(buf); got != wantData {
		t.Fatalf("received %q, want %q", got, wantData)
	}
	conn.Close()
	if err := session.Wait(); err != nil {
		t.Fatalf("guest write: %v\nstderr: %q", err, stderr.String())
	}
}
