package vz

import (
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// VirtualMachineConfiguration defines the configuration of a VirtualMachine.
//
// The following properties must be configured before creating a virtual machine:
//   - bootLoader
//
// The configuration of devices is often done in two parts:
// - Device configuration
// - Device attachment
//
// The device configuration defines the characteristics of the emulated hardware device.
// For example, for a network device, the device configuration defines the type of network adapter present
// in the virtual machine and its MAC address.
//
// The device attachment defines the host machine's resources that are exposed by the virtual device.
// For example, for a network device, the device attachment can be virtual network interface with a NAT
// to the real network.
//
// Creating a virtual machine using the Virtualization framework requires the app to have the "com.apple.security.virtualization" entitlement.
// A VirtualMachineConfiguration is considered invalid if the application does not have the entitlement.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration?language=objc
type VirtualMachineConfiguration struct {
	*pointer

	networkDeviceConfiguration []*VirtioNetworkDeviceConfiguration
	storageDeviceConfiguration []StorageDeviceConfiguration
	usbControllerConfiguration []USBControllerConfiguration
}

// NewVirtualMachineConfiguration creates a new configuration.
//
//   - bootLoader parameter is used when the virtual machine starts.
//   - cpu parameter is The number of CPUs must be a value between
//     VZVirtualMachineConfiguration.minimumAllowedCPUCount and VZVirtualMachineConfiguration.maximumAllowedCPUCount.
//   - memorySize parameter represents memory size in bytes.
//     The memory size must be a multiple of a 1 megabyte (1024 * 1024 bytes) between
//     VZVirtualMachineConfiguration.minimumAllowedMemorySize and VZVirtualMachineConfiguration.maximumAllowedMemorySize.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtualMachineConfiguration(bootLoader BootLoader, cpu uint, memorySize uint64) (*VirtualMachineConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	config := &VirtualMachineConfiguration{
		pointer: vzbridge.NewVZVirtualMachineConfiguration(
			bootLoader,
			uint32(cpu),
			uint64(memorySize),
		),
	}
	return config, nil
}

// Validate the configuration.
//
// Return true if the configuration is valid.
// If error is not nil, assigned with the validation error if the validation failed.
func (v *VirtualMachineConfiguration) Validate() (bool, error) {
	var nserrPtr unsafe.Pointer
	ret := vzbridge.VZVirtualMachineConfiguration_ValidateWithError(v, &nserrPtr)
	err := newNSError(nserrPtr)
	if err != nil {
		return false, err
	}
	return (bool)(ret), nil
}

// SetEntropyDevicesVirtualMachineConfiguration sets list of entropy devices. Empty by default.
func (v *VirtualMachineConfiguration) SetEntropyDevicesVirtualMachineConfiguration(cs []*VirtioEntropyDeviceConfiguration) {
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetEntropyDevices(v, array)
}

// SetMemoryBalloonDevicesVirtualMachineConfiguration sets list of memory balloon devices. Empty by default.
func (v *VirtualMachineConfiguration) SetMemoryBalloonDevicesVirtualMachineConfiguration(cs []MemoryBalloonDeviceConfiguration) {
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetMemoryBalloonDevices(v, array)
}

// SetNetworkDevicesVirtualMachineConfiguration sets list of network adapters. Empty by default.
func (v *VirtualMachineConfiguration) SetNetworkDevicesVirtualMachineConfiguration(cs []*VirtioNetworkDeviceConfiguration) {
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetNetworkDevices(v, array)
	v.networkDeviceConfiguration = cs
}

// NetworkDevices return the list of network device configuration set in this virtual machine configuration.
// Return an empty array if no network device configuration is set.
func (v *VirtualMachineConfiguration) NetworkDevices() []*VirtioNetworkDeviceConfiguration {
	return v.networkDeviceConfiguration
}

// SetSerialPortsVirtualMachineConfiguration sets list of serial ports. Empty by default.
func (v *VirtualMachineConfiguration) SetSerialPortsVirtualMachineConfiguration(cs []*VirtioConsoleDeviceSerialPortConfiguration) {
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetSerialPorts(v, array)
}

// SetSocketDevicesVirtualMachineConfiguration sets list of socket devices. Empty by default.
func (v *VirtualMachineConfiguration) SetSocketDevicesVirtualMachineConfiguration(cs []SocketDeviceConfiguration) {
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetSocketDevices(v, array)
}

// SocketDevices return the list of socket device configuration configured in this virtual machine configuration.
// Return an empty array if no socket device configuration is set.
func (v *VirtualMachineConfiguration) SocketDevices() []SocketDeviceConfiguration {
	ptrs := nativeArray(vzbridge.VZVirtualMachineConfiguration_SocketDevices(v))
	socketDevices := make([]SocketDeviceConfiguration, len(ptrs))
	for i, ptr := range ptrs {
		socketDevices[i] = &VirtioSocketDeviceConfiguration{pointer: ptr}
	}
	return socketDevices
}

// SetStorageDevicesVirtualMachineConfiguration sets list of disk devices. Empty by default.
func (v *VirtualMachineConfiguration) SetStorageDevicesVirtualMachineConfiguration(cs []StorageDeviceConfiguration) {
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetStorageDevices(v, array)
	v.storageDeviceConfiguration = cs
}

// StorageDevices return the list of storage device configuration configured in this virtual machine configuration.
// Return an empty array if no storage device configuration is set.
func (v *VirtualMachineConfiguration) StorageDevices() []StorageDeviceConfiguration {
	return v.storageDeviceConfiguration
}

// SetDirectorySharingDevicesVirtualMachineConfiguration sets list of directory sharing devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetDirectorySharingDevicesVirtualMachineConfiguration(cs []DirectorySharingDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetDirectorySharingDevices(v, array)
}

// SetPlatformVirtualMachineConfiguration sets the hardware platform to use. Defaults to GenericPlatformConfiguration.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetPlatformVirtualMachineConfiguration(c PlatformConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	vzbridge.VZVirtualMachineConfiguration_SetPlatform(v, c)
}

// SetGraphicsDevicesVirtualMachineConfiguration sets list of graphics devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetGraphicsDevicesVirtualMachineConfiguration(cs []GraphicsDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetGraphicsDevices(v, array)
}

// SetPointingDevicesVirtualMachineConfiguration sets list of pointing devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetPointingDevicesVirtualMachineConfiguration(cs []PointingDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetPointingDevices(v, array)
}

// SetKeyboardsVirtualMachineConfiguration sets list of keyboards. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetKeyboardsVirtualMachineConfiguration(cs []KeyboardConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetKeyboards(v, array)
}

// SetAudioDevicesVirtualMachineConfiguration sets list of audio devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetAudioDevicesVirtualMachineConfiguration(cs []AudioDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetAudioDevices(v, array)
}

// SetConsoleDevicesVirtualMachineConfiguration sets list of console devices. Empty by default.
//
// This is only supported on macOS 13 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetConsoleDevicesVirtualMachineConfiguration(cs []ConsoleDeviceConfiguration) {
	if err := macOSAvailable(13); err != nil {
		return
	}
	array := nativeObjectArray(cs)
	vzbridge.VZVirtualMachineConfiguration_SetConsoleDevices(v, array)
}

// SetUSBControllerConfiguration sets list of USB controllers. Empty by default.
//
// This is only supported on macOS 15 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetUSBControllersVirtualMachineConfiguration(us []USBControllerConfiguration) {
	if err := macOSAvailable(15); err != nil {
		return
	}
	array := nativeObjectArray(us)
	vzbridge.VZVirtualMachineConfiguration_SetUsbControllers(v, array)
	v.usbControllerConfiguration = us
}

// USBControllers return the list of usb controller configuration configured in this virtual machine configuration.
// Return an empty array if no usb controller configuration is set.
func (v *VirtualMachineConfiguration) USBControllers() []USBControllerConfiguration {
	return v.usbControllerConfiguration
}

// VirtualMachineConfigurationMinimumAllowedMemorySize returns minimum
// amount of memory required by virtual machines.
func VirtualMachineConfigurationMinimumAllowedMemorySize() uint64 {
	return vzbridge.VZVirtualMachineConfiguration_MinimumAllowedMemorySize()
}

// VirtualMachineConfigurationMaximumAllowedMemorySize returns maximum
// amount of memory allowed for a virtual machine.
func VirtualMachineConfigurationMaximumAllowedMemorySize() uint64 {
	return vzbridge.VZVirtualMachineConfiguration_MaximumAllowedMemorySize()
}

// VirtualMachineConfigurationMinimumAllowedCPUCount returns minimum
// number of CPUs for a virtual machine.
func VirtualMachineConfigurationMinimumAllowedCPUCount() uint {
	return uint(vzbridge.VZVirtualMachineConfiguration_MinimumAllowedCPUCount())
}

// VirtualMachineConfigurationMaximumAllowedCPUCount returns maximum
// number of CPUs for a virtual machine.
func VirtualMachineConfigurationMaximumAllowedCPUCount() uint {
	return uint(vzbridge.VZVirtualMachineConfiguration_MaximumAllowedCPUCount())
}
