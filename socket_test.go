package vz_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v2"
)

func TestVirtioSocketListener(t *testing.T) {
	container := newVirtualizationMachine(t)
	defer container.Close()

	vm := container.VirtualMachine

	socketDevice := vm.SocketDevices()[0] // already tested in newVirtualizationMachine

	wantData := "hello"
	done := make(chan struct{})

	listener, err := vz.NewVirtioSocketListener(func(conn *vz.VirtioSocketConnection, err error) {
		defer close(done)

		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
			return
		}
		defer conn.Close()

		buf := make([]byte, len(wantData))
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("failed to read data: %v", err)
			return
		}
		got := string(buf[:n])

		if wantData != got {
			t.Errorf("want %q but got %q", wantData, got)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	port := 43218
	socketDevice.SetSocketListenerForPort(listener, uint32(port))

	session := container.NewSession(t)
	var buf bytes.Buffer
	session.Stderr = &buf
	cmd := fmt.Sprintf("echo %s | socat - VSOCK-CONNECT:2:%d", wantData, port)
	if err := session.Run(cmd); err != nil {
		t.Fatalf("failed to write data to vsock: %v\nstderr: %q", err, buf)
	}
	session.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout connection handling after accepted")
	}

	socketDevice.RemoveSocketListenerForPort(listener, uint32(port))
}
