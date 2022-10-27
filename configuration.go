package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

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
	cpuCount   uint
	memorySize uint64
	pointer
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
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewVirtualMachineConfiguration(bootLoader BootLoader, cpu uint, memorySize uint64) (*VirtualMachineConfiguration, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	config := &VirtualMachineConfiguration{
		cpuCount:   cpu,
		memorySize: memorySize,
		pointer: pointer{
			ptr: C.newVZVirtualMachineConfiguration(
				bootLoader.Ptr(),
				C.uint(cpu),
				C.ulonglong(memorySize),
			),
		},
	}
	runtime.SetFinalizer(config, func(self *VirtualMachineConfiguration) {
		self.Release()
	})
	return config, nil
}

// Validate the configuration.
//
// Return true if the configuration is valid.
// If error is not nil, assigned with the validation error if the validation failed.
func (v *VirtualMachineConfiguration) Validate() (bool, error) {
	nserr := newNSErrorAsNil()
	nserrPtr := nserr.Ptr()
	ret := C.validateVZVirtualMachineConfiguration(v.Ptr(), &nserrPtr)
	err := newNSError(nserrPtr)
	if err != nil {
		return false, err
	}
	return (bool)(ret), nil
}

// SetEntropyDevicesVirtualMachineConfiguration sets list of entropy devices. Empty by default.
func (v *VirtualMachineConfiguration) SetEntropyDevicesVirtualMachineConfiguration(cs []*VirtioEntropyDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setEntropyDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetMemoryBalloonDevicesVirtualMachineConfiguration sets list of memory balloon devices. Empty by default.
func (v *VirtualMachineConfiguration) SetMemoryBalloonDevicesVirtualMachineConfiguration(cs []MemoryBalloonDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setMemoryBalloonDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetNetworkDevicesVirtualMachineConfiguration sets list of network adapters. Empty by default.
func (v *VirtualMachineConfiguration) SetNetworkDevicesVirtualMachineConfiguration(cs []*VirtioNetworkDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setNetworkDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// NetworkDevices return the list of network device configuration set in this virtual machine configuration.
// Return an empty array if no network device configuration is set.
func (v *VirtualMachineConfiguration) NetworkDevices() []*VirtioNetworkDeviceConfiguration {
	nsArray := &NSArray{
		pointer: pointer{
			ptr: C.networkDevicesVZVirtualMachineConfiguration(v.Ptr()),
		},
	}
	ptrs := nsArray.ToPointerSlice()
	networkDevices := make([]*VirtioNetworkDeviceConfiguration, len(ptrs))
	for i, ptr := range ptrs {
		networkDevices[i] = newVirtioNetworkDeviceConfiguration(ptr)
	}
	return networkDevices
}

// SetSerialPortsVirtualMachineConfiguration sets list of serial ports. Empty by default.
func (v *VirtualMachineConfiguration) SetSerialPortsVirtualMachineConfiguration(cs []*VirtioConsoleDeviceSerialPortConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setSerialPortsVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetSocketDevicesVirtualMachineConfiguration sets list of socket devices. Empty by default.
func (v *VirtualMachineConfiguration) SetSocketDevicesVirtualMachineConfiguration(cs []SocketDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setSocketDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SocketDevices return the list of socket device configuration configured in this virtual machine configuration.
// Return an empty array if no socket device configuration is set.
func (v *VirtualMachineConfiguration) SocketDevices() []SocketDeviceConfiguration {
	nsArray := &NSArray{
		pointer: pointer{
			ptr: C.socketDevicesVZVirtualMachineConfiguration(v.Ptr()),
		},
	}
	ptrs := nsArray.ToPointerSlice()
	socketDevices := make([]SocketDeviceConfiguration, len(ptrs))
	for i, ptr := range ptrs {
		socketDevices[i] = newVirtioSocketDeviceConfiguration(ptr)
	}
	return socketDevices
}

// SetStorageDevicesVirtualMachineConfiguration sets list of disk devices. Empty by default.
func (v *VirtualMachineConfiguration) SetStorageDevicesVirtualMachineConfiguration(cs []StorageDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setStorageDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetDirectorySharingDevicesVirtualMachineConfiguration sets list of directory sharing devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetDirectorySharingDevicesVirtualMachineConfiguration(cs []DirectorySharingDeviceConfiguration) {
	if macosMajorVersionLessThan(12) {
		return
	}
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setDirectorySharingDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetPlatformVirtualMachineConfiguration sets the hardware platform to use. Defaults to GenericPlatformConfiguration.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetPlatformVirtualMachineConfiguration(c PlatformConfiguration) {
	if macosMajorVersionLessThan(12) {
		return
	}
	C.setPlatformVZVirtualMachineConfiguration(v.Ptr(), c.Ptr())
}

// SetGraphicsDevicesVirtualMachineConfiguration sets list of graphics devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetGraphicsDevicesVirtualMachineConfiguration(cs []GraphicsDeviceConfiguration) {
	if macosMajorVersionLessThan(12) {
		return
	}
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setGraphicsDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetPointingDevicesVirtualMachineConfiguration sets list of pointing devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetPointingDevicesVirtualMachineConfiguration(cs []PointingDeviceConfiguration) {
	if macosMajorVersionLessThan(12) {
		return
	}
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setPointingDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetKeyboardsVirtualMachineConfiguration sets list of keyboards. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetKeyboardsVirtualMachineConfiguration(cs []KeyboardConfiguration) {
	if macosMajorVersionLessThan(12) {
		return
	}
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setKeyboardsVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// SetAudioDevicesVirtualMachineConfiguration sets list of audio devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetAudioDevicesVirtualMachineConfiguration(cs []AudioDeviceConfiguration) {
	if macosMajorVersionLessThan(12) {
		return
	}
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setAudioDevicesVZVirtualMachineConfiguration(v.Ptr(), array.Ptr())
}

// VirtualMachineConfigurationMinimumAllowedMemorySize returns minimum
// amount of memory required by virtual machines.
func VirtualMachineConfigurationMinimumAllowedMemorySize() uint64 {
	return uint64(C.minimumAllowedMemorySizeVZVirtualMachineConfiguration())
}

// VirtualMachineConfigurationMaximumAllowedMemorySize returns maximum
// amount of memory allowed for a virtual machine.
func VirtualMachineConfigurationMaximumAllowedMemorySize() uint64 {
	return uint64(C.maximumAllowedMemorySizeVZVirtualMachineConfiguration())
}

// VirtualMachineConfigurationMinimumAllowedCPUCount returns minimum
// number of CPUs for a virtual machine.
func VirtualMachineConfigurationMinimumAllowedCPUCount() uint {
	return uint(C.minimumAllowedCPUCountVZVirtualMachineConfiguration())
}

// VirtualMachineConfigurationMaximumAllowedCPUCount returns maximum
// number of CPUs for a virtual machine.
func VirtualMachineConfigurationMaximumAllowedCPUCount() uint {
	return uint(C.maximumAllowedCPUCountVZVirtualMachineConfiguration())
}
