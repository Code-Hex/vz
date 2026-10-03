package vz

import (
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// ConsoleDeviceConfiguration interface for an console device configuration.
type ConsoleDeviceConfiguration interface {
	objc.NSObject

	consoleDeviceConfiguration()
}

type baseConsoleDeviceConfiguration struct{}

func (*baseConsoleDeviceConfiguration) consoleDeviceConfiguration() {}

// VirtioConsoleDeviceConfiguration is Virtio Console Device.
type VirtioConsoleDeviceConfiguration struct {
	*pointer
	portsPtr *objc.Pointer

	*baseConsoleDeviceConfiguration

	consolePorts map[int]*VirtioConsolePortConfiguration
}

var _ ConsoleDeviceConfiguration = (*VirtioConsoleDeviceConfiguration)(nil)

// NewVirtioConsoleDeviceConfiguration creates a new VirtioConsoleDeviceConfiguration.
func NewVirtioConsoleDeviceConfiguration() (*VirtioConsoleDeviceConfiguration, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}
	config := &VirtioConsoleDeviceConfiguration{
		pointer:      vzbridge.VZVirtioConsoleDeviceConfiguration_Init(),
		consolePorts: make(map[int]*VirtioConsolePortConfiguration),
	}
	config.portsPtr = vzbridge.VZVirtioConsoleDeviceConfiguration_Ports(config)
	return config, nil
}

// MaximumPortCount returns the maximum number of ports allocated by this device.
// The default is the number of ports attached to this device.
func (v *VirtioConsoleDeviceConfiguration) MaximumPortCount() uint32 {
	return uint32(vzbridge.VZVirtioConsolePortConfigurationArray_MaximumPortCount(v.portsPtr))
}

func (v *VirtioConsoleDeviceConfiguration) SetVirtioConsolePortConfiguration(idx int, portConfig *VirtioConsolePortConfiguration) {
	vzbridge.VZVirtioConsolePortConfigurationArray_SetObject_AtIndexedSubscript(v.portsPtr, portConfig, uint64(int32(idx)))

	// to mark as currently reachable.
	// This ensures that the object is not freed, and its finalizer is not run
	v.consolePorts[idx] = portConfig
}

type ConsolePortConfiguration interface {
	objc.NSObject

	consolePortConfiguration()
}

type baseConsolePortConfiguration struct{}

func (*baseConsolePortConfiguration) consolePortConfiguration() {}

// VirtioConsolePortConfiguration is Virtio Console Port
//
// A console port is a two-way communication channel between a host VZSerialPortAttachment and
// a virtual machine console port. One or more console ports are attached to a Virtio console device.
type VirtioConsolePortConfiguration struct {
	*pointer

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
		vzbridge.VZVirtioConsolePortConfiguration_SetName(vcpc, vzbridge.NSString_StringWithUTF8String(name))
		vcpc.name = name
	}
}

// WithVirtioConsolePortConfigurationIsConsole sets the console port may be marked
// for use as the system console. The default is false.
func WithVirtioConsolePortConfigurationIsConsole(isConsole bool) NewVirtioConsolePortConfigurationOption {
	return func(vcpc *VirtioConsolePortConfiguration) {
		vzbridge.VZVirtioConsolePortConfiguration_SetIsConsole(vcpc, bool(isConsole))
		vcpc.isConsole = isConsole
	}
}

// WithVirtioConsolePortConfigurationAttachment sets the console port attachment.
// The default is nil.
func WithVirtioConsolePortConfigurationAttachment(attachment SerialPortAttachment) NewVirtioConsolePortConfigurationOption {
	return func(vcpc *VirtioConsolePortConfiguration) {
		vzbridge.VZConsolePortConfiguration_SetAttachment(vcpc, attachment)
		vcpc.attachment = attachment
	}
}

// NewVirtioConsolePortConfiguration creates a new VirtioConsolePortConfiguration.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewVirtioConsolePortConfiguration(opts ...NewVirtioConsolePortConfigurationOption) (*VirtioConsolePortConfiguration, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}
	vcpc := &VirtioConsolePortConfiguration{
		pointer: vzbridge.VZVirtioConsolePortConfiguration_Init(),
	}
	for _, optFunc := range opts {
		optFunc(vcpc)
	}
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
