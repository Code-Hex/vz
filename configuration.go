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
// - bootLoader parameter is used when the virtual machine starts.
// - cpu parameter is The number of CPUs must be a value between
//     VZVirtualMachineConfiguration.minimumAllowedCPUCount and VZVirtualMachineConfiguration.maximumAllowedCPUCount.
// - memorySize parameter represents memory size in bytes.
//    The memory size must be a multiple of a 1 megabyte (1024 * 1024 bytes) between
//    VZVirtualMachineConfiguration.minimumAllowedMemorySize and VZVirtualMachineConfiguration.maximumAllowedMemorySize.
func NewVirtualMachineConfiguration(bootLoader BootLoader, cpu uint, memorySize uint64) *VirtualMachineConfiguration {
	config := &VirtualMachineConfiguration{
		cpuCount:   cpu,
		memorySize: memorySize,
		pointer: newPointer(C.newVZVirtualMachineConfiguration(
			bootLoader.ptr(),
			C.uint(cpu),
			C.ulonglong(memorySize),
		),
		),
	}
	runtime.SetFinalizer(config, func(self *VirtualMachineConfiguration) {
		self.release()
	})
	return config
}

// Validate the configuration.
//
// Return true if the configuration is valid.
// If error is not nil, assigned with the validation error if the validation failed.
func (v *VirtualMachineConfiguration) Validate() (bool, error) {
	nserr := newNSErrorAsNil()
	nserrPtr := nserr.ptr()
	ret := C.validateVZVirtualMachineConfiguration(v.ptr(), &nserrPtr)
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
	C.setEntropyDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetMemoryBalloonDevicesVirtualMachineConfiguration sets list of memory balloon devices. Empty by default.
func (v *VirtualMachineConfiguration) SetMemoryBalloonDevicesVirtualMachineConfiguration(cs []MemoryBalloonDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setMemoryBalloonDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetNetworkDevicesVirtualMachineConfiguration sets list of network adapters. Empty by default.
func (v *VirtualMachineConfiguration) SetNetworkDevicesVirtualMachineConfiguration(cs []*VirtioNetworkDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setNetworkDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetSerialPortsVirtualMachineConfiguration sets list of serial ports. Empty by default.
func (v *VirtualMachineConfiguration) SetSerialPortsVirtualMachineConfiguration(cs []*VirtioConsoleDeviceSerialPortConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setSerialPortsVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetSocketDevicesVirtualMachineConfiguration sets list of socket devices. Empty by default.
func (v *VirtualMachineConfiguration) SetSocketDevicesVirtualMachineConfiguration(cs []SocketDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setSocketDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetStorageDevicesVirtualMachineConfiguration sets list of disk devices. Empty by default.
func (v *VirtualMachineConfiguration) SetStorageDevicesVirtualMachineConfiguration(cs []StorageDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setStorageDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetDirectorySharingDevicesVirtualMachineConfiguration sets list of directory sharing devices. Empty by default.
func (v *VirtualMachineConfiguration) SetDirectorySharingDevicesVirtualMachineConfiguration(cs []DirectorySharingDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setDirectorySharingDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetPlatformVirtualMachineConfiguration sets the hardware platform to use. Defaults to GenericPlatformConfiguration.
func (v *VirtualMachineConfiguration) SetPlatformVirtualMachineConfiguration(c PlatformConfiguration) {
	C.setPlatformVZVirtualMachineConfiguration(v.ptr(), c.ptr())
}

// SetGraphicsDevicesVirtualMachineConfiguration sets list of graphics devices. Empty by default.
func (v *VirtualMachineConfiguration) SetGraphicsDevicesVirtualMachineConfiguration(cs []GraphicsDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setGraphicsDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetPointingDevicesVirtualMachineConfiguration sets list of pointing devices. Empty by default.
func (v *VirtualMachineConfiguration) SetPointingDevicesVirtualMachineConfiguration(cs []PointingDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setPointingDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetKeyboardsVirtualMachineConfiguration sets list of keyboards. Empty by default.
func (v *VirtualMachineConfiguration) SetKeyboardsVirtualMachineConfiguration(cs []KeyboardConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setKeyboardsVZVirtualMachineConfiguration(v.ptr(), array.ptr())
}

// SetAudioDevicesVirtualMachineConfiguration sets list of audio devices. Empty by default.
func (v *VirtualMachineConfiguration) SetAudioDevicesVirtualMachineConfiguration(cs []AudioDeviceConfiguration) {
	ptrs := make([]NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setAudioDevicesVZVirtualMachineConfiguration(v.ptr(), array.ptr())
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
