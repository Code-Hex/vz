package vz_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func TestRunningGraphicsDisplay(t *testing.T) {
	if vz.Available(14) {
		t.Skip("requires macOS 14")
	}
	vm := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
		graphics, err := vz.NewVirtioGraphicsDeviceConfiguration()
		if err != nil {
			return err
		}
		scanout, err := vz.NewVirtioGraphicsScanoutConfiguration(800, 600)
		if err != nil {
			return err
		}
		graphics.SetScanouts(scanout)
		config.SetGraphicsDevicesVirtualMachineConfiguration([]vz.GraphicsDeviceConfiguration{graphics})
		return nil
	})
	t.Cleanup(func() {
		if err := vm.Shutdown(); err != nil {
			t.Error(err)
		}
	})
	displays := vm.GraphicsDevices()[0].Displays()
	if len(displays) != 1 {
		t.Fatalf("got %d displays, want 1", len(displays))
	}
	display := displays[0]
	if err := display.ReconfigureWithSizeInPixels(1024, 768); err != nil {
		t.Fatal(err)
	}
	if width, height := display.SizeInPixels(); width != 1024 || height != 768 {
		t.Fatalf("size = %gx%g, want 1024x768", width, height)
	}
	configuration, err := vz.NewVirtioGraphicsScanoutConfiguration(1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	if err := display.ReconfigureWithConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	if width, height := display.SizeInPixels(); width != 1280 || height != 720 {
		t.Fatalf("size = %gx%g, want 1280x720", width, height)
	}
	err = display.ReconfigureWithSizeInPixels(-1, 0)
	var nativeError *vz.NSError
	if !errors.As(err, &nativeError) {
		t.Fatalf("invalid size error = %v, want NSError", err)
	}
	if err := vm.conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	defer vm.conn.SetDeadline(time.Time{})
	session := vm.NewSession(t)
	defer session.Close()
	if output, err := session.CombinedOutput("echo display-alive"); err != nil || string(output) != "display-alive\n" {
		t.Fatalf("guest response = %q, error = %v", output, err)
	}
}
