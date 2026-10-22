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
