package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_13.h"
*/
import "C"
import (
	"runtime"
	"unsafe"
)

// ConsoleDeviceConfiguration interface for an console device configuration.
type ConsoleDeviceConfiguration interface {
	NSObject

	consoleDeviceConfiguration()
}

type baseConsoleDeviceConfiguration struct{}

func (*baseConsoleDeviceConfiguration) consoleDeviceConfiguration() {}

// VirtioConsoleDeviceConfiguration is Virtio Console Device.
type VirtioConsoleDeviceConfiguration struct {
	pointer
	portsPtr unsafe.Pointer

	*baseConsoleDeviceConfiguration

	consolePorts map[int]*VirtioConsolePortConfiguration
}

var _ ConsoleDeviceConfiguration = (*VirtioConsoleDeviceConfiguration)(nil)

// NewVirtioConsoleDeviceConfiguration creates a new VirtioConsoleDeviceConfiguration.
func NewVirtioConsoleDeviceConfiguration() (*VirtioConsoleDeviceConfiguration, error) {
	if macosMajorVersionLessThan(13) {
		return nil, ErrUnsupportedOSVersion
	}
	config := &VirtioConsoleDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioConsoleDeviceConfiguration(),
		},
		consolePorts: make(map[int]*VirtioConsolePortConfiguration),
	}
	config.portsPtr = C.portsVZVirtioConsoleDeviceConfiguration(config.Ptr())

	runtime.SetFinalizer(config, func(self *VirtioConsoleDeviceConfiguration) {
		self.Release()
	})
	return config, nil
}

// MaximumPortCount returns the maximum number of ports allocated by this device.
// The default is the number of ports attached to this device.
func (v *VirtioConsoleDeviceConfiguration) MaximumPortCount() uint32 {
	return uint32(C.maximumPortCountVZVirtioConsolePortConfigurationArray(v.portsPtr))
}

func (v *VirtioConsoleDeviceConfiguration) SetVirtioConsolePortConfiguration(idx int, portConfig *VirtioConsolePortConfiguration) {
	C.setObjectAtIndexedSubscriptVZVirtioConsolePortConfigurationArray(
		v.portsPtr,
		portConfig.Ptr(),
		C.int(idx),
	)

	// to mark as currently reachable.
	// This ensures that the object is not freed, and its finalizer is not run
	v.consolePorts[idx] = portConfig
}

type ConsolePortConfiguration interface {
	NSObject

	consolePortConfiguration()
}

type baseConsolePortConfiguration struct{}

func (*baseConsolePortConfiguration) consolePortConfiguration() {}

// VirtioConsolePortConfiguration is Virtio Console Port
//
// A console port is a two-way communication channel between a host VZSerialPortAttachment and
// a virtual machine console port. One or more console ports are attached to a Virtio console device.
type VirtioConsolePortConfiguration struct {
	pointer

	*baseConsolePortConfiguration

	isConsole  bool
	name       string
	attachment SerialPortAttachment
}

var _ ConsolePortConfiguration = (*VirtioConsolePortConfiguration)(nil)

// NewVirtioConsolePortConfigurationOption is an option type to initialize a new VirtioConsolePortConfiguration
type NewVirtioConsolePortConfigurationOption func(*VirtioConsolePortConfiguration)

// WithVirtioConsolePortConfigurationName sets the console port's name.
// The default behavior is to not use a name unless set.
func WithVirtioConsolePortConfigurationName(name string) NewVirtioConsolePortConfigurationOption {
	return func(vcpc *VirtioConsolePortConfiguration) {
		consolePortName := charWithGoString(name)
		defer consolePortName.Free()
		C.setNameVZVirtioConsolePortConfiguration(
			vcpc.Ptr(),
			consolePortName.CString(),
		)
		vcpc.name = name
	}
}

// WithVirtioConsolePortConfigurationIsConsole sets the console port may be marked
// for use as the system console. The default is false.
func WithVirtioConsolePortConfigurationIsConsole(isConsole bool) NewVirtioConsolePortConfigurationOption {
	return func(vcpc *VirtioConsolePortConfiguration) {
		C.setIsConsoleVZVirtioConsolePortConfiguration(
			vcpc.Ptr(),
			C.bool(isConsole),
		)
		vcpc.isConsole = isConsole
	}
}

// WithVirtioConsolePortConfigurationAttachment sets the console port attachment.
// The default is nil.
func WithVirtioConsolePortConfigurationAttachment(attachment SerialPortAttachment) NewVirtioConsolePortConfigurationOption {
	return func(vcpc *VirtioConsolePortConfiguration) {
		C.setAttachmentVZVirtioConsolePortConfiguration(
			vcpc.Ptr(),
			attachment.Ptr(),
		)
		vcpc.attachment = attachment
	}
}

// NewVirtioConsolePortConfiguration creates a new VirtioConsolePortConfiguration.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewVirtioConsolePortConfiguration(opts ...NewVirtioConsolePortConfigurationOption) (*VirtioConsolePortConfiguration, error) {
	if macosMajorVersionLessThan(13) {
		return nil, ErrUnsupportedOSVersion
	}
	vcpc := &VirtioConsolePortConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioConsolePortConfiguration(),
		},
	}
	for _, optFunc := range opts {
		optFunc(vcpc)
	}
	runtime.SetFinalizer(vcpc, func(self *VirtioConsolePortConfiguration) {
		self.Release()
	})
	return vcpc, nil
}

// Name returns the console port's name.
func (v *VirtioConsolePortConfiguration) Name() string { return v.name }

// IsConsole returns the console port may be marked for use as the system console.
func (v *VirtioConsolePortConfiguration) IsConsole() bool { return v.isConsole }

// Attachment returns the console port attachment.
func (v *VirtioConsolePortConfiguration) Attachment() SerialPortAttachment {
	return v.attachment
}
