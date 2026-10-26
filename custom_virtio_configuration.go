package vz

import (
	"errors"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// VirtioFeatureSet contains Virtio feature bits 0 through 31 in Subset0 and
// bits 32 through 63 in Subset1.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofeatureset?language=objc
type VirtioFeatureSet struct {
	// Subset0 contains feature bits 0 through 31.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzvirtiofeatureset/subset0?language=objc
	Subset0 uint32
	// Subset1 contains feature bits 32 through 63.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzvirtiofeatureset/subset1?language=objc
	Subset1 uint32
}

// CustomVirtioDeviceConfiguration describes a device implemented by a custom provider.
// It requires macOS 27 or newer.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration?language=objc
type CustomVirtioDeviceConfiguration struct {
	*pointer
}

// CustomVirtioDeviceConfigurationOption configures a custom Virtio device.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration?language=objc
type CustomVirtioDeviceConfigurationOption func(*CustomVirtioDeviceConfiguration) error

// NewCustomVirtioDeviceConfiguration creates a custom Virtio device configuration.
// The IDs identify the device to the guest. queueCount specifies its virtqueues.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration?language=objc
func NewCustomVirtioDeviceConfiguration(deviceID uint16, pciClassID, pciSubclassID uint8, queueCount uint16, options ...CustomVirtioDeviceConfigurationOption) (*CustomVirtioDeviceConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	c := &CustomVirtioDeviceConfiguration{
		pointer: vzbridge.VZCustomVirtioDeviceConfiguration_New(),
	}
	vzbridge.VZCustomVirtioDeviceConfiguration_SetDeviceID(c, deviceID)
	vzbridge.VZCustomVirtioDeviceConfiguration_SetPCIClassID(c, pciClassID)
	vzbridge.VZCustomVirtioDeviceConfiguration_SetPCISubclassID(c, pciSubclassID)
	vzbridge.VZCustomVirtioDeviceConfiguration_SetVirtioQueueCount(c, queueCount)
	for _, option := range options {
		if err := option(c); err != nil {
			objc.Release(c)
			return nil, err
		}
	}
	return c, nil
}

// WithCustomVirtioMandatoryFeatures sets features the guest must accept.
// The framework always enables VIRTIO_F_VERSION_1.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/mandatoryfeatures?language=objc
func WithCustomVirtioMandatoryFeatures(features VirtioFeatureSet) CustomVirtioDeviceConfigurationOption {
	return func(c *CustomVirtioDeviceConfiguration) error {
		native := vzbridge.VZCustomVirtioDeviceConfiguration_MandatoryFeatures(c)
		defer objc.Release(native)
		vzbridge.VZVirtioFeatureSet_SetSubset0(native, features.Subset0)
		vzbridge.VZVirtioFeatureSet_SetSubset1(native, features.Subset1)
		return nil
	}
}

// WithCustomVirtioOptionalFeatures sets features the guest may accept.
// The framework may enable additional features internally.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/optionalfeatures?language=objc
func WithCustomVirtioOptionalFeatures(features VirtioFeatureSet) CustomVirtioDeviceConfigurationOption {
	return func(c *CustomVirtioDeviceConfiguration) error {
		native := vzbridge.VZCustomVirtioDeviceConfiguration_OptionalFeatures(c)
		defer objc.Release(native)
		vzbridge.VZVirtioFeatureSet_SetSubset0(native, features.Subset0)
		vzbridge.VZVirtioFeatureSet_SetSubset1(native, features.Subset1)
		return nil
	}
}

// WithCustomVirtioDeviceSpecificConfiguration supplies the initial device configuration.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/devicespecificconfiguration?language=objc
func WithCustomVirtioDeviceSpecificConfiguration(configuration *VirtioDeviceSpecificConfiguration) CustomVirtioDeviceConfigurationOption {
	return func(c *CustomVirtioDeviceConfiguration) error {
		var native objc.NSObject
		if configuration != nil {
			native = configuration
		}
		vzbridge.VZCustomVirtioDeviceConfiguration_SetDeviceSpecificConfiguration(c, native)
		return nil
	}
}

// WithCustomVirtioSharedMemoryRegions sets the memory regions exposed to the guest.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/sharedmemoryregions?language=objc
func WithCustomVirtioSharedMemoryRegions(regions ...*VirtioSharedMemoryRegionConfiguration) CustomVirtioDeviceConfigurationOption {
	return func(c *CustomVirtioDeviceConfiguration) error {
		for _, region := range regions {
			if region == nil || objc.Ptr(region) == nil {
				return errors.New("vz: nil custom Virtio shared memory region configuration")
			}
		}
		array := nativeObjectArray(regions)
		defer objc.Release(array)
		vzbridge.VZCustomVirtioDeviceConfiguration_SetSharedMemoryRegions(c, array)
		return nil
	}
}

// DeviceID returns the Virtio device ID.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/deviceid?language=objc
func (c *CustomVirtioDeviceConfiguration) DeviceID() uint16 {
	return vzbridge.VZCustomVirtioDeviceConfiguration_DeviceID(c)
}

// PCIClassID returns the PCI class ID.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/pciclassid?language=objc
func (c *CustomVirtioDeviceConfiguration) PCIClassID() uint8 {
	return vzbridge.VZCustomVirtioDeviceConfiguration_PCIClassID(c)
}

// PCISubclassID returns the PCI subclass ID.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/pcisubclassid?language=objc
func (c *CustomVirtioDeviceConfiguration) PCISubclassID() uint8 {
	return vzbridge.VZCustomVirtioDeviceConfiguration_PCISubclassID(c)
}

// VirtioQueueCount returns the configured number of virtqueues.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/virtioqueuecount?language=objc
func (c *CustomVirtioDeviceConfiguration) VirtioQueueCount() uint16 {
	return vzbridge.VZCustomVirtioDeviceConfiguration_VirtioQueueCount(c)
}

// MandatoryFeatures returns the features required from the guest driver.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/mandatoryfeatures?language=objc
func (c *CustomVirtioDeviceConfiguration) MandatoryFeatures() VirtioFeatureSet {
	native := vzbridge.VZCustomVirtioDeviceConfiguration_MandatoryFeatures(c)
	defer objc.Release(native)
	return VirtioFeatureSet{
		Subset0: vzbridge.VZVirtioFeatureSet_Subset0(native),
		Subset1: vzbridge.VZVirtioFeatureSet_Subset1(native),
	}
}

// OptionalFeatures returns the features offered for optional negotiation.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/optionalfeatures?language=objc
func (c *CustomVirtioDeviceConfiguration) OptionalFeatures() VirtioFeatureSet {
	native := vzbridge.VZCustomVirtioDeviceConfiguration_OptionalFeatures(c)
	defer objc.Release(native)
	return VirtioFeatureSet{
		Subset0: vzbridge.VZVirtioFeatureSet_Subset0(native),
		Subset1: vzbridge.VZVirtioFeatureSet_Subset1(native),
	}
}

// VirtioDeviceSpecificConfiguration contains serialized device configuration data.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiodevicespecificconfiguration?language=objc
type VirtioDeviceSpecificConfiguration struct {
	*pointer
}

// NewVirtioDeviceSpecificConfiguration copies data into a device configuration.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiodevicespecificconfiguration/init(configurationdata:)?language=objc
func NewVirtioDeviceSpecificConfiguration(data []byte) (*VirtioDeviceSpecificConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	native := vzbridge.NSData_DataWithBytes_Length(unsafe.Pointer(unsafe.SliceData(data)), uint64(len(data)))
	runtime.KeepAlive(data)
	defer objc.Release(native)
	return &VirtioDeviceSpecificConfiguration{
		pointer: vzbridge.VZVirtioDeviceSpecificConfiguration_InitWithConfigurationData(native),
	}, nil
}

// ConfigurationData returns a copy of the configuration bytes.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiodevicespecificconfiguration/configurationdata?language=objc
func (c *VirtioDeviceSpecificConfiguration) ConfigurationData() []byte {
	return objcutil.Bytes(vzbridge.VZVirtioDeviceSpecificConfiguration_ConfigurationData(c))
}

// VirtioSharedMemoryRegionConfiguration reserves a region for host memory mappings.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregionconfiguration?language=objc
type VirtioSharedMemoryRegionConfiguration struct {
	*pointer
}

// NewVirtioSharedMemoryRegionConfiguration creates a region identified by regionID.
// The size is in bytes. This requires macOS 27 or newer.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregionconfiguration/init(regionid:size:)?language=objc
func NewVirtioSharedMemoryRegionConfiguration(regionID uint8, size uint64) (*VirtioSharedMemoryRegionConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	return &VirtioSharedMemoryRegionConfiguration{
		pointer: vzbridge.VZVirtioSharedMemoryRegionConfiguration_InitWithRegionID_Size(regionID, size),
	}, nil
}

// RegionID returns the region's device-specific identifier.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregionconfiguration/regionid?language=objc
func (c *VirtioSharedMemoryRegionConfiguration) RegionID() uint8 {
	return vzbridge.VZVirtioSharedMemoryRegionConfiguration_RegionID(c)
}

// Size returns the region size in bytes.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregionconfiguration/size?language=objc
func (c *VirtioSharedMemoryRegionConfiguration) Size() uint64 {
	return vzbridge.VZVirtioSharedMemoryRegionConfiguration_Size(c)
}

// MaximumAllowedVirtioSharedMemoryRegionCount returns the framework's region limit.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/maximumallowedsharedmemoryregioncount?language=objc
func MaximumAllowedVirtioSharedMemoryRegionCount() (uint, error) {
	if err := macOSAvailable(27); err != nil {
		return 0, err
	}
	return uint(vzbridge.VZCustomVirtioDeviceConfiguration_MaximumAllowedSharedMemoryRegionCount()), nil
}

// WithCustomVirtioDevices sets the custom devices in a virtual machine.
// This requires macOS 27 or newer.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration/customvirtiodevices?language=objc
func WithCustomVirtioDevices(devices ...*CustomVirtioDeviceConfiguration) VirtualMachineConfigurationOption {
	return func(v *VirtualMachineConfiguration) error {
		if err := macOSAvailable(27); err != nil {
			return err
		}
		for _, device := range devices {
			if device == nil || objc.Ptr(device) == nil {
				return errors.New("vz: nil custom Virtio device configuration")
			}
		}
		array := nativeObjectArray(devices)
		defer objc.Release(array)
		vzbridge.VZVirtualMachineConfiguration_SetCustomVirtioDevices(v, array)
		return nil
	}
}

// DeviceSpecificConfiguration returns the configured data, or nil if absent.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/devicespecificconfiguration?language=objc
func (c *CustomVirtioDeviceConfiguration) DeviceSpecificConfiguration() *VirtioDeviceSpecificConfiguration {
	native := vzbridge.VZCustomVirtioDeviceConfiguration_DeviceSpecificConfiguration(c)
	if native == nil {
		return nil
	}
	return &VirtioDeviceSpecificConfiguration{pointer: native}
}

// SharedMemoryRegions returns the configured shared memory regions.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/sharedmemoryregions?language=objc
func (c *CustomVirtioDeviceConfiguration) SharedMemoryRegions() []*VirtioSharedMemoryRegionConfiguration {
	objects := nativeArray(vzbridge.VZCustomVirtioDeviceConfiguration_SharedMemoryRegions(c))
	regions := make([]*VirtioSharedMemoryRegionConfiguration, len(objects))
	for i, object := range objects {
		regions[i] = &VirtioSharedMemoryRegionConfiguration{pointer: object}
	}
	return regions
}

// SupportsSaveRestore reports whether the provider implements state saving and restoration.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/supportssaverestore?language=objc
func (c *CustomVirtioDeviceConfiguration) SupportsSaveRestore() bool {
	return vzbridge.VZCustomVirtioDeviceConfiguration_SupportsSaveRestore(c)
}

// CustomVirtioDevices returns the configured custom Virtio devices.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration/customvirtiodevices?language=objc
func (v *VirtualMachineConfiguration) CustomVirtioDevices() ([]*CustomVirtioDeviceConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	objects := nativeArray(vzbridge.VZVirtualMachineConfiguration_CustomVirtioDevices(v))
	devices := make([]*CustomVirtioDeviceConfiguration, len(objects))
	for i, object := range objects {
		devices[i] = &CustomVirtioDeviceConfiguration{pointer: object}
	}
	return devices, nil
}
