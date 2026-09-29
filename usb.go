package vz

import (
	"runtime"

	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

// NewUSBMassStorageDevice initialize the runtime USB Mass Storage device object.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
func NewUSBMassStorageDevice(config *USBMassStorageDeviceConfiguration) (USBDevice, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}
	ptr := vzbridge.NewVZUSBMassStorageDeviceWithConfiguration(config)
	return newUSBDevice(ptr), nil
}

// USBControllerConfiguration for a usb controller configuration.
type USBControllerConfiguration interface {
	objc.NSObject

	usbControllerConfiguration()
}

type baseUSBControllerConfiguration struct{}

func (*baseUSBControllerConfiguration) usbControllerConfiguration() {}

// XHCIControllerConfiguration is a configuration of the USB XHCI controller.
//
// This configuration creates a USB XHCI controller device for the guest.
// see: https://developer.apple.com/documentation/virtualization/vzxhcicontrollerconfiguration?language=objc
type XHCIControllerConfiguration struct {
	*pointer

	*baseUSBControllerConfiguration
}

var _ USBControllerConfiguration = (*XHCIControllerConfiguration)(nil)

// NewXHCIControllerConfiguration creates a new XHCIControllerConfiguration.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
func NewXHCIControllerConfiguration() (*XHCIControllerConfiguration, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}

	config := &XHCIControllerConfiguration{
		pointer: vzbridge.NewVZXHCIControllerConfiguration(),
	}
	return config, nil
}

// USBController is representing a USB controller in a virtual machine.
type USBController struct {
	vm *VirtualMachine
	*pointer
}

func newUSBController(ptr *objc.Pointer, vm *VirtualMachine) *USBController {
	return &USBController{
		vm:      vm,
		pointer: ptr,
	}
}

// Attach attaches a USB device.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
//
// If the device is successfully attached to the controller, it will appear in the usbDevices property,
// its usbController property will be set to point to the USB controller that it is attached to
// and completion handler will return nil.
// If the device was previously attached to this or another USB controller, attach function will fail
// with the `vz.ErrorDeviceAlreadyAttached`. If the device cannot be initialized correctly, attach
// function will fail with `vz.ErrorDeviceInitializationFailure`.
func (u *USBController) Attach(device USBDevice) error {
	if err := macOSAvailable(15); err != nil {
		return err
	}
	handle, errCh := nativeCompletion()
	defer runtime.KeepAlive(u)
	defer runtime.KeepAlive(device)
	vzbridge.AttachDeviceVZUSBController(
		u,
		device,
		uint64(handle),
	)
	return <-errCh
}

// Detach detaches a USB device.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
//
// If the device is successfully detached from the controller, it will disappear from the usbDevices property,
// its usbController property will be set to nil and completion handler will return nil.
// If the device wasn't attached to the controller at the time of calling detach method, it will fail
// with the `vz.ErrorDeviceNotFound` error.
func (u *USBController) Detach(device USBDevice) error {
	if err := macOSAvailable(15); err != nil {
		return err
	}
	handle, errCh := nativeCompletion()
	defer runtime.KeepAlive(u)
	defer runtime.KeepAlive(device)
	vzbridge.DetachDeviceVZUSBController(
		u,
		device,
		uint64(handle),
	)
	return <-errCh
}

// USBDevices return a list of USB devices attached to controller.
//
// This is only supported on macOS 15 and newer, nil will
// be returned on older versions.
func (u *USBController) USBDevices() []USBDevice {
	if err := macOSAvailable(15); err != nil {
		return nil
	}
	pointers := nativeArray(vzbridge.UsbDevicesVZUSBController(u))
	usbDevices := make([]USBDevice, len(pointers))
	for i, ptr := range pointers {
		usbDevices[i] = newUSBDevice(ptr)
	}

	return usbDevices
}

// USBDevice is an interface that represents a USB device in a VM.
type USBDevice interface {
	objc.NSObject

	UUID() string

	usbDevice()
}

func newUSBDevice(ptr *objc.Pointer) *usbDevice {
	return &usbDevice{
		pointer: ptr,
	}
}

type usbDevice struct {
	*pointer
}

func (*usbDevice) usbDevice() {}

var _ USBDevice = (*usbDevice)(nil)

// UUID returns the device UUID.
func (u *usbDevice) UUID() string {
	return nativeString(vzbridge.GetUUIDUSBDevice(u))
}
