package vz

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestRuntimeGraphicsDisplay(t *testing.T) {
	if macOSAvailable(14) != nil {
		t.Skip("requires macOS 14")
	}
	display, released := retainedGraphicsDisplay(t)
	runtime.GC()
	vzbridge.Drain()
	if released() {
		t.Fatal("machine released while display remains live")
	}
	width, height := display.SizeInPixels()
	if width != 800 || height != 600 {
		t.Fatalf("size = %gx%g, want 800x600", width, height)
	}
	if err := display.ReconfigureWithConfiguration(nil); err == nil {
		t.Fatal("nil configuration accepted")
	}
	runtime.KeepAlive(display)
	display = nil
	awaitNativeBridge(t, released)
}

func retainedGraphicsDisplay(t *testing.T) (*GraphicsDisplay, func() bool) {
	t.Helper()
	graphics, err := NewVirtioGraphicsDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	scanout, err := NewVirtioGraphicsScanoutConfiguration(800, 600)
	if err != nil {
		t.Fatal(err)
	}
	graphics.SetScanouts(scanout)
	config := newTestConfig(t)
	config.SetGraphicsDevicesVirtualMachineConfiguration([]GraphicsDeviceConfiguration{graphics})
	machine, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	devices := machine.GraphicsDevices()
	if len(devices) != 1 {
		t.Fatalf("got %d graphics devices, want 1", len(devices))
	}
	displays := devices[0].Displays()
	if len(displays) != 1 {
		t.Fatalf("got %d displays, want 1", len(displays))
	}
	return displays[0], nativeMachineReleased(machine)
}
