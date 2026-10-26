package vz

import (
	"errors"
	"runtime"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// NewUSBMassStorageDevice initialize the runtime USB Mass Storage device object.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
func NewUSBMassStorageDevice(config *USBMassStorageDeviceConfiguration) (USBDevice, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}
	ptr := vzbridge.VZUSBMassStorageDevice_InitWithConfiguration(config)
	return &usbDevice{pointer: ptr}, nil
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
	devices []USBDeviceConfiguration

	*baseUSBControllerConfiguration
}

var _ USBControllerConfiguration = (*XHCIControllerConfiguration)(nil)

// NewXHCIControllerConfiguration creates a new XHCIControllerConfiguration.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzxhcicontrollerconfiguration?language=objc
func NewXHCIControllerConfiguration(options ...XHCIControllerConfigurationOption) (*XHCIControllerConfiguration, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}

	config := &XHCIControllerConfiguration{
		pointer: vzbridge.VZXHCIControllerConfiguration_Init(),
	}
	for _, option := range options {
		if err := option(config); err != nil {
			objc.Release(config)
			return nil, err
		}
	}
	return config, nil
}

// USBDeviceConfiguration describes a USB device attached when the VM starts.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbdeviceconfiguration?language=objc
type USBDeviceConfiguration interface {
	objc.NSObject
	usbDeviceConfiguration()
}

// USBDeviceConfigurationOption configures a USB device at creation.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbdeviceconfiguration?language=objc
type USBDeviceConfigurationOption func(USBDeviceConfiguration) error

// WithUSBDeviceUUID sets the device UUID. Before restoring a virtual machine,
// use the UUID returned by USBDevice.UUID when the machine was saved.
// If omitted, the framework generates a UUID.
//
// This requires macOS 15 or later. It returns an error if uuid is invalid.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbdeviceconfiguration/uuid?language=objc
func WithUSBDeviceUUID(uuid string) USBDeviceConfigurationOption {
	return func(configuration USBDeviceConfiguration) error {
		if err := macOSAvailable(15); err != nil {
			return err
		}
		if !validUTF8CString(uuid) {
			return errors.New("vz: invalid USB device UUID")
		}
		value := vzbridge.NSString_StringWithUTF8String(uuid)
		defer objc.Release(value)
		nativeUUID := vzbridge.NSUUID_InitWithUUIDString(value)
		if objc.Ptr(nativeUUID) == nil {
			return errors.New("vz: invalid USB device UUID")
		}
		defer objc.Release(nativeUUID)

		switch config := configuration.(type) {
		case *USBMassStorageDeviceConfiguration:
			vzbridge.VZUSBMassStorageDeviceConfiguration_SetUuid(config, nativeUUID)
		case *USBPassthroughDeviceConfiguration:
			vzbridge.VZUSBPassthroughDeviceConfiguration_SetUuid(config, nativeUUID)
		default:
			return errors.New("vz: unsupported USB device configuration")
		}
		return nil
	}
}

// XHCIControllerConfigurationOption configures an XHCI controller at creation.
//
// see: https://developer.apple.com/documentation/virtualization/vzxhcicontrollerconfiguration?language=objc
type XHCIControllerConfigurationOption func(*XHCIControllerConfiguration) error

// WithUSBDevices sets the USB devices attached when the VM starts.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbcontrollerconfiguration/usbdevices?language=objc
func WithUSBDevices(devices ...USBDeviceConfiguration) XHCIControllerConfigurationOption {
	devices = append([]USBDeviceConfiguration(nil), devices...)
	return func(configuration *XHCIControllerConfiguration) error {
		for _, device := range devices {
			switch d := device.(type) {
			case *USBMassStorageDeviceConfiguration:
				if d == nil {
					return errors.New("vz: nil USB device configuration")
				}
			case *USBPassthroughDeviceConfiguration:
				if d == nil {
					return errors.New("vz: nil USB device configuration")
				}
			}
			if objc.Ptr(device) == nil {
				return errors.New("vz: nil USB device configuration")
			}
		}
		array := nativeObjectArray(devices)
		defer objc.Release(array)
		vzbridge.VZUSBControllerConfiguration_SetUsbDevices(configuration, array)
		configuration.devices = devices
		return nil
	}
}

// USBDevices returns the USB devices configured for startup.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbcontrollerconfiguration/usbdevices?language=objc
func (c *XHCIControllerConfiguration) USBDevices() []USBDeviceConfiguration {
	return append([]USBDeviceConfiguration(nil), c.devices...)
}

// USBController is representing a USB controller in a virtual machine.
type USBController struct {
	vm *VirtualMachine
	*pointer
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
	pointers := nativeArray(vzbridge.VZUSBController_UsbDevices(u))
	usbDevices := make([]USBDevice, len(pointers))
	for i, ptr := range pointers {
		usbDevices[i] = &usbDevice{pointer: ptr}
	}

	return usbDevices
}

// USBDevice is an interface that represents a USB device in a VM.
type USBDevice interface {
	objc.NSObject

	UUID() string

	// USBController returns the controller this device is attached to, or nil if detached.
	// The returned controller does not keep the virtual machine alive.
	// This is only supported on macOS 15 and newer.
	USBController() *USBController

	usbDevice()
}

type usbDevice struct {
	*pointer
}

func (*usbDevice) usbDevice() {}

var _ USBDevice = (*usbDevice)(nil)

// UUID returns the device UUID.
func (u *usbDevice) UUID() string {
	return objcutil.String(vzbridge.GetUUIDUSBDevice(u))
}

// USBController returns the controller this device is attached to, or nil if detached.
// The returned controller does not keep the virtual machine alive.
// This is only supported on macOS 15 and newer.
func (u *usbDevice) USBController() *USBController {
	if macOSAvailable(15) != nil {
		return nil
	}
	ptr := vzbridge.VZUSBMassStorageDevice_UsbController(u)
	if ptr == nil {
		return nil
	}
	return &USBController{pointer: ptr}
}
