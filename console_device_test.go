package vz

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestRuntimeConsolePort(t *testing.T) {
	if macOSAvailable(13) != nil {
		t.Skip("requires macOS 13")
	}
	config := newTestConfig(t)
	console, err := NewVirtioConsoleDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	portConfig, err := NewVirtioConsolePortConfiguration(WithVirtioConsolePortConfigurationName("control"))
	if err != nil {
		t.Fatal(err)
	}
	console.SetVirtioConsolePortConfiguration(3, portConfig)
	config.SetConsoleDevicesVirtualMachineConfiguration([]ConsoleDeviceConfiguration{console})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	devices := vm.ConsoleDevices()
	if len(devices) != 1 {
		t.Fatalf("console count = %d, want 1", len(devices))
	}
	ports := devices[0].Ports()
	if len(ports) != 1 || ports[3] == nil {
		t.Fatalf("ports = %v, want only port 3", ports)
	}
	port := ports[3]
	if got := port.Name(); got != "control" {
		t.Fatalf("name = %q", got)
	}
	if got := port.Attachment(); got != nil {
		t.Fatalf("initial attachment = %T", got)
	}
	attachment, err := NewFileSerialPortAttachment(filepath.Join(t.TempDir(), "console"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []SerialPortAttachment{attachment, nil, attachment} {
		if err := port.SetAttachment(want); err != nil {
			t.Fatal(err)
		}
		got := port.Attachment()
		if objc.Ptr(got) != objc.Ptr(want) {
			t.Fatalf("attachment = %v, want %v", got, want)
		}
		if got != nil {
			objc.Release(got)
		}
	}
	released := nativeMachineReleased(vm)
	vm = nil
	devices = nil
	ports = nil
	for range 3 {
		runtime.GC()
		vzbridge.Drain()
	}
	if released() {
		t.Fatal("VM released while port is live")
	}
	if got := port.Name(); got != "control" {
		t.Fatalf("name after GC = %q", got)
	}
	runtime.KeepAlive(port)
	port = nil
	awaitNativeBridge(t, released)
}

func TestRuntimeConsolePortSpiceAttachment(t *testing.T) {
	if macOSAvailable(13) != nil {
		t.Skip("requires macOS 13")
	}
	attachment, err := NewSpiceAgentPortAttachment()
	if err != nil {
		t.Fatal(err)
	}
	name, err := SpiceAgentPortAttachmentName()
	if err != nil {
		t.Fatal(err)
	}
	portConfig, err := NewVirtioConsolePortConfiguration(
		WithVirtioConsolePortConfigurationName(name),
		WithVirtioConsolePortConfigurationAttachment(attachment),
	)
	if err != nil {
		t.Fatal(err)
	}
	console, err := NewVirtioConsoleDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	console.SetVirtioConsolePortConfiguration(0, portConfig)
	config := newTestConfig(t)
	config.SetConsoleDevicesVirtualMachineConfiguration([]ConsoleDeviceConfiguration{console})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	port := vm.ConsoleDevices()[0].Ports()[0]
	for _, enabled := range []bool{true, false} {
		attachment.SetSharesClipboard(enabled)
		got, ok := port.Attachment().(*SpiceAgentPortAttachment)
		if !ok {
			t.Fatalf("attachment is not a SpiceAgentPortAttachment")
		}
		defer objc.Release(got)
		if got.SharesClipboard() != enabled {
			t.Fatalf("shares clipboard = %v, want %v", got.SharesClipboard(), enabled)
		}
		got.SetSharesClipboard(!enabled)
		if attachment.SharesClipboard() != !enabled {
			t.Fatalf("original attachment shares clipboard = %v, want %v", attachment.SharesClipboard(), !enabled)
		}
	}
	if err := port.SetAttachment(nil); err != nil {
		t.Fatal(err)
	}
	if got := port.Attachment(); got != nil {
		t.Fatalf("disconnected attachment = %T, want nil", got)
	}
}
