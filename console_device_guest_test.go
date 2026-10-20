package vz_test

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func TestRuntimeConsoleAttachmentGuest(t *testing.T) {
	if vz.Available(13) {
		t.Skip("requires macOS 13")
	}
	container := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
		console, err := vz.NewVirtioConsoleDeviceConfiguration()
		if err != nil {
			return err
		}
		port, err := vz.NewVirtioConsolePortConfiguration(vz.WithVirtioConsolePortConfigurationName("control"))
		if err != nil {
			return err
		}
		console.SetVirtioConsolePortConfiguration(1, port)
		config.SetConsoleDevicesVirtualMachineConfiguration([]vz.ConsoleDeviceConfiguration{console})
		return nil
	})
	defer func() {
		if err := container.Shutdown(); err != nil {
			t.Error(err)
		}
	}()
	if err := container.conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	session := container.NewSession(t)
	output, err := session.Output(`for p in /sys/class/virtio-ports/*; do if [ "$(cat "$p/name")" = control ]; then echo /dev/$(basename "$p"); fi; done`)
	session.Close()
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(output))
	if !strings.HasPrefix(path, "/dev/vport") {
		t.Fatalf("guest port path = %q", path)
	}
	port := container.ConsoleDevices()[0].Ports()[1]
	for _, message := range []string{"first", "second"} {
		read, input, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer read.Close()
		defer input.Close()
		output, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		defer write.Close()
		attachment, err := vz.NewFileHandleSerialPortAttachment(read, write)
		if err != nil {
			t.Fatal(err)
		}
		if err := port.SetAttachment(attachment); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := port.SetAttachment(nil); err != nil {
				t.Error(err)
			}
		}()
		if got := port.Attachment(); got == nil {
			t.Fatal("port remained disconnected")
		} else if _, ok := got.(*vz.FileHandleSerialPortAttachment); !ok {
			t.Fatalf("attachment = %T, want file handle attachment", got)
		}
		session := container.NewSession(t)
		defer session.Close()
		if err := output.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := session.Start(fmt.Sprintf("timeout 5 sh -c 'exec 3<> %s; printf R >&3; IFS= read -r line <&3 && printf %%s \"$line\" >&3'", path)); err != nil {
			t.Fatal(err)
		}
		var ready [1]byte
		if _, err := io.ReadFull(output, ready[:]); err != nil {
			t.Fatal(err)
		}
		if ready[0] != 'R' {
			t.Fatalf("ready = %q, want R", ready)
		}
		if err := input.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(input, message); err != nil {
			t.Fatal(err)
		}
		data := make([]byte, len(message))
		if _, err := io.ReadFull(output, data); err != nil {
			t.Fatal(err)
		}
		if string(data) != message {
			t.Fatalf("echo = %q, want %q", data, message)
		}
		if err := session.Wait(); err != nil {
			t.Fatal(err)
		}
		session.Close()
		if err := port.SetAttachment(nil); err != nil {
			t.Fatal(err)
		}
		if port.Attachment() != nil {
			t.Fatal("port remained connected")
		}
	}
}
