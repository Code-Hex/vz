//go:build darwin

package vz

import (
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/objc"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestRuntimeNotificationsRelease(t *testing.T) {
	if macOSAvailable(14) != nil {
		t.Skip("requires macOS 14")
	}
	config := newTestConfig(t)
	device, err := NewVirtioConsoleDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewVirtioConsolePortConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	device.SetVirtioConsolePortConfiguration(3, port)
	config.SetConsoleDevicesVirtualMachineConfiguration([]ConsoleDeviceConfiguration{device})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	console := vm.ConsolePortChangedNotify()
	graphics := vm.GraphicsDisplayChangedNotify()
	stopped := vm.GuestStoppedNotify()
	if console != vm.ConsolePortChangedNotify() || graphics != vm.GraphicsDisplayChangedNotify() || stopped != vm.GuestStoppedNotify() {
		t.Fatal("notification methods returned different channels")
	}
	emit := func() {
		onMachineQueue(t, vm, func() {
			machine := pureobjc.ID(uintptr(objc.Ptr(vm)))
			devices := machine.Send(pureobjc.RegisterName("consoleDevices"))
			device := devices.Send(pureobjc.RegisterName("objectAtIndex:"), uintptr(0))
			delegate := device.Send(pureobjc.RegisterName("delegate"))
			ports := device.Send(pureobjc.RegisterName("ports"))
			port := ports.Send(pureobjc.RegisterName("objectAtIndexedSubscript:"), uintptr(3))
			for range 20 {
				delegate.Send(pureobjc.RegisterName("consoleDevice:didOpenPort:"), device, port)
				delegate.Send(pureobjc.RegisterName("consoleDevice:didClosePort:"), device, port)
			}
			machine.Send(pureobjc.RegisterName("guestDidStopVirtualMachine:"), machine)
		})
	}
	emit()
	for i := range 40 {
		select {
		case event := <-console:
			want := ConsolePortEvent{DeviceIndex: 0, PortIndex: 3, Open: i%2 == 0}
			if event != want {
				t.Fatalf("event %d = %+v, want %+v", i, event, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("missing console event %d", i)
		}
	}
	emit()
	runtime.KeepAlive(vm)
	vm = nil
	awaitNativeBridge(t, func() bool {
		select {
		case _, open := <-graphics:
			return !open
		default:
			return false
		}
	})
	select {
	case _, open := <-console:
		if open {
			t.Fatal("console notifications remained open")
		}
	default:
		t.Fatal("console notifications remained open")
	}
	select {
	case _, open := <-stopped:
		if open {
			t.Fatal("guest shutdown notifications remained open")
		}
	default:
		t.Fatal("guest shutdown notifications remained open")
	}
}
