package vz_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
)

func TestUSBDeviceController(t *testing.T) {
	if vz.Available(15) {
		t.Skip("requires macOS 15")
	}
	path := filepath.Join(t.TempDir(), "usb.img")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(1024 * 1024); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	attachment, err := vz.NewDiskImageStorageDeviceAttachment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewUSBMassStorageDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	device, err := vz.NewUSBMassStorageDevice(config)
	if err != nil {
		t.Fatal(err)
	}
	if device.USBController() != nil {
		t.Fatal("new device has a controller")
	}
	vm := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
		controller, err := vz.NewXHCIControllerConfiguration()
		if err != nil {
			return err
		}
		config.SetUSBControllersVirtualMachineConfiguration([]vz.USBControllerConfiguration{controller})
		return nil
	})
	t.Cleanup(func() {
		if err := vm.Shutdown(); err != nil {
			t.Error(err)
		}
	})
	controllers := vm.USBControllers()
	if len(controllers) != 1 {
		t.Fatalf("got %d controllers, want 1", len(controllers))
	}
	if err := controllers[0].Attach(device); err != nil {
		t.Fatal(err)
	}
	controller := device.USBController()
	if controller == nil || objc.Ptr(controller) != objc.Ptr(controllers[0]) {
		t.Fatal("attached device did not return its controller")
	}
	devices := controller.USBDevices()
	if len(devices) != 1 || devices[0].UUID() != device.UUID() {
		t.Fatal("returned controller did not contain the attached device")
	}
	if err := controller.Detach(device); err != nil {
		t.Fatal(err)
	}
	if device.USBController() != nil || devices[0].USBController() != nil {
		t.Fatal("detached device still has a controller")
	}
}
