//go:build darwin && arm64

package vzbridge

import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

func NewVZMacHardwareModelWithBytes(bytes unsafe.Pointer, length int32) *objc.Pointer {
	if length < 0 || (length > 0 && bytes == nil) {
		return nil
	}
	data := NSData_DataWithBytes_Length(bytes, uint64(length))
	defer objc.Release(data)
	return VZMacHardwareModel_InitWithDataRepresentation(data)
}

func NewVZMacMachineIdentifierWithBytes(bytes unsafe.Pointer, length int32) *objc.Pointer {
	if length < 0 || (length > 0 && bytes == nil) {
		return nil
	}
	data := NSData_DataWithBytes_Length(bytes, uint64(length))
	defer objc.Release(data)
	return VZMacMachineIdentifier_InitWithDataRepresentation(data)
}

func NewVZMacAuxiliaryStorage(path string) *objc.Pointer {
	value := NSString_StringWithUTF8String(path)
	defer objc.Release(value)
	url := NSURL_FileURLWithPath(value)
	defer objc.Release(url)
	return VZMacAuxiliaryStorage_InitWithContentsOfURL(url)
}

func NewVZMacAuxiliaryStorageWithCreating(path string, hardwareModel objc.NSObject, errorOut *unsafe.Pointer) *objc.Pointer {
	value := NSString_StringWithUTF8String(path)
	defer objc.Release(value)
	url := NSURL_FileURLWithPath(value)
	defer objc.Release(url)
	return VZMacAuxiliaryStorage_InitCreatingStorageAtURL_HardwareModel_Options_Error(
		url, hardwareModel, VZMacAuxiliaryStorageInitializationOptionAllowOverwrite, errorOut,
	)
}
