package vz_test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func TestVirtioSocketListener(t *testing.T) {
	container := newVirtualizationMachine(t)
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	vm := container.VirtualMachine

	socketDevice := vm.SocketDevices()[0] // already tested in newVirtualizationMachine

	const readTimeout = 3 * time.Second
	port := 43218
	wantData := "hello"
	done := make(chan struct{})

	listener, err := socketDevice.Listen(uint32(port))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		defer close(done)

		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("failed to accept connection: %v", err)
			return
		}
		defer conn.Close()

		destPort := conn.(*vz.VirtioSocketConnection).DestinationPort()
		if port != int(destPort) {
			t.Errorf("want destination port %d but got %d", destPort, port)
			return
		}

		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			t.Errorf("failed to set read deadline: %v", err)
			return
		}

		buf := make([]byte, len(wantData))
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Errorf("failed to read data: %v", err)
			return
		}
		got := string(buf)

		if wantData != got {
			t.Errorf("want %q but got %q", wantData, got)
		}
	}()

	session := container.NewSession(t)
	var buf bytes.Buffer
	session.Stderr = &buf
	cmd := fmt.Sprintf("{ printf he; sleep 0.2; printf llo; } | socat - VSOCK-CONNECT:2:%d", port)
	if err := session.Run(cmd); err != nil {
		t.Fatalf("failed to write data to vsock: %v\nstderr: %q", err, buf.String())
	}
	session.Close()

	select {
	case <-done:
	case <-time.After(readTimeout):
		t.Fatalf("timeout connection handling after accepted")
	}
}
