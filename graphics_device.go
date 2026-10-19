package vz

import (
	"errors"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// GraphicsDisplayConfiguration configures a graphics display.
// Its concrete type must match the display's original configuration.
type GraphicsDisplayConfiguration interface {
	objc.NSObject
	graphicsDisplayConfiguration()
}

// GraphicsDevice is a graphics device attached to a virtual machine.
// Obtain it from VirtualMachine.GraphicsDevices.
type GraphicsDevice struct {
	*pointer
	vm *VirtualMachine
}

// GraphicsDisplay is a display attached to a virtual machine.
// Obtain it from GraphicsDevice.Displays.
type GraphicsDisplay struct {
	*pointer
	vm *VirtualMachine
}

// GraphicsDevices returns the graphics devices configured on the VM.
// It returns nil on macOS versions older than 14.
func (v *VirtualMachine) GraphicsDevices() []*GraphicsDevice {
	if macOSAvailable(14) != nil {
		return nil
	}
	pointers := nativeArray(vzbridge.VZVirtualMachine_GraphicsDevices(v))
	devices := make([]*GraphicsDevice, len(pointers))
	for i, ptr := range pointers {
		devices[i] = &GraphicsDevice{
			pointer: ptr,
			vm:      v,
		}
	}
	return devices
}

// Displays returns the displays configured on the graphics device.
func (d *GraphicsDevice) Displays() []*GraphicsDisplay {
	pointers := nativeArray(vzbridge.VZGraphicsDevice_Displays(d))
	displays := make([]*GraphicsDisplay, len(pointers))
	for i, ptr := range pointers {
		displays[i] = &GraphicsDisplay{
			pointer: ptr,
			vm:      d.vm,
		}
	}
	return displays
}

// SizeInPixels returns the display's current width and height in pixels.
func (d *GraphicsDisplay) SizeInPixels() (width, height float64) {
	var size [2]float64
	vzbridge.GraphicsDisplaySize(d, unsafe.Pointer(&size))
	return size[0], size[1]
}

// ReconfigureWithSizeInPixels requests a new display size in pixels.
// A nil error means the request was accepted. The guest may ignore the new size.
// This requires macOS 14 or newer.
func (d *GraphicsDisplay) ReconfigureWithSizeInPixels(width, height float64) error {
	if err := macOSAvailable(14); err != nil {
		return err
	}
	var errorPointer unsafe.Pointer
	vzbridge.ReconfigureGraphicsDisplaySize(d, width, height, &errorPointer)
	if err := objcutil.NewNSError(errorPointer); err != nil {
		return err
	}
	return nil
}

// ReconfigureWithConfiguration requests a new display configuration.
// The configuration type must match the display's original configuration.
// A nil error means the request was accepted. The guest may ignore the changes.
// This requires macOS 14 or newer.
func (d *GraphicsDisplay) ReconfigureWithConfiguration(configuration GraphicsDisplayConfiguration) error {
	if err := macOSAvailable(14); err != nil {
		return err
	}
	if configuration == nil {
		return errors.New("graphics display configuration must not be nil")
	}
	var errorPointer unsafe.Pointer
	vzbridge.VZGraphicsDisplay_ReconfigureWithConfiguration_Error(d, configuration, &errorPointer)
	if err := objcutil.NewNSError(errorPointer); err != nil {
		return err
	}
	return nil
}
