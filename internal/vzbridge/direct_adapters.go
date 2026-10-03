//go:build darwin

package vzbridge

import (
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
)

func NewVZGenericMachineIdentifierWithBytes(bytes unsafe.Pointer, length int32) *objc.Pointer {
	if bytes == nil || length < 0 {
		return nil
	}
	data := NSData_DataWithBytes_Length(bytes, uint64(length))
	defer objc.Release(data)
	return VZGenericMachineIdentifier_InitWithDataRepresentation(data)
}

func NewVZVirtualMachineConfiguration(bootLoader objc.NSObject, cpuCount uint32, memorySize uint64) *objc.Pointer {
	config := VZVirtualMachineConfiguration_New()
	VZVirtualMachineConfiguration_SetBootLoader(config, bootLoader)
	VZVirtualMachineConfiguration_SetCPUCount(config, uint64(cpuCount))
	VZVirtualMachineConfiguration_SetMemorySize(config, memorySize)
	return config
}

func NewVZVirtioFileSystemDeviceConfiguration(value string, errorOut *unsafe.Pointer) *objc.Pointer {
	tag := NSString_StringWithUTF8String(value)
	defer objc.Release(tag)
	if !VZVirtioFileSystemDeviceConfiguration_ValidateTag_Error(tag, errorOut) {
		return nil
	}
	return VZVirtioFileSystemDeviceConfiguration_InitWithTag(tag)
}

func SetBlockDeviceIdentifierVZVirtioBlockDeviceConfiguration(config objc.NSObject, value string, errorOut *unsafe.Pointer) {
	identifier := NSString_StringWithUTF8String(value)
	defer objc.Release(identifier)
	if VZVirtioBlockDeviceConfiguration_ValidateBlockDeviceIdentifier_Error(identifier, errorOut) {
		VZVirtioBlockDeviceConfiguration_SetBlockDeviceIdentifier(config, identifier)
	}
}

func VZBridgedNetworkInterface_localizedDisplayName(networkInterface objc.NSObject) *objc.Pointer {
	if name := VZBridgedNetworkInterface_LocalizedDisplayName(networkInterface); name != nil {
		return name
	}
	return VZBridgedNetworkInterface_Identifier(networkInterface)
}

func NewVZDiskImageStorageDeviceAttachmentWithCacheAndSyncMode(path string, readOnly bool, cachingMode int32, synchronizationMode int32, errorOut *unsafe.Pointer) *objc.Pointer {
	cachingValid := cachingMode == VZDiskImageCachingModeAutomatic || cachingMode == VZDiskImageCachingModeCached || cachingMode == VZDiskImageCachingModeUncached
	syncValid := synchronizationMode == VZDiskImageSynchronizationModeFull || synchronizationMode == VZDiskImageSynchronizationModeFsync || synchronizationMode == VZDiskImageSynchronizationModeNone
	if !cachingValid || !syncValid {
		SetInvalidDiskModeError(errorOut)
		return nil
	}
	value := NSString_StringWithUTF8String(path)
	defer objc.Release(value)
	url := NSURL_FileURLWithPath(value)
	defer objc.Release(url)
	return VZDiskImageStorageDeviceAttachment_InitWithURL_ReadOnly_CachingMode_SynchronizationMode_Error(url, readOnly, int64(cachingMode), int64(synchronizationMode), errorOut)
}
