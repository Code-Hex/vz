package vz

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_11.h"
# include "virtualization_12.h"
# include "virtualization_13.h"
*/
import "C"
import (
	"os"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// DirectorySharingDeviceConfiguration for a directory sharing device configuration.
type DirectorySharingDeviceConfiguration interface {
	objc.NSObject

	directorySharingDeviceConfiguration()
}

type baseDirectorySharingDeviceConfiguration struct{}

func (*baseDirectorySharingDeviceConfiguration) directorySharingDeviceConfiguration() {}

var _ DirectorySharingDeviceConfiguration = (*VirtioFileSystemDeviceConfiguration)(nil)

// VirtioFileSystemDeviceConfiguration is a configuration of a Virtio file system device.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofilesystemdeviceconfiguration?language=objc
type VirtioFileSystemDeviceConfiguration struct {
	*pointer

	*baseDirectorySharingDeviceConfiguration
}

// NewVirtioFileSystemDeviceConfiguration create a new VirtioFileSystemDeviceConfiguration.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewVirtioFileSystemDeviceConfiguration(tag string) (*VirtioFileSystemDeviceConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	tagChar := charWithGoString(tag)
	defer tagChar.Free()

	nserrPtr := newNSErrorAsNil()
	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: objc.NewPointer(
			C.newVZVirtioFileSystemDeviceConfiguration(tagChar.CString(), &nserrPtr),
		),
	}
	if err := newNSError(nserrPtr); err != nil {
		return nil, err
	}
	objc.SetFinalizer(fsdConfig, func(self *VirtioFileSystemDeviceConfiguration) {
		objc.Release(self)
	})
	return fsdConfig, nil
}

// SetDirectoryShare sets the directory share associated with this configuration.
func (c *VirtioFileSystemDeviceConfiguration) SetDirectoryShare(share DirectoryShare) {
	C.setVZVirtioFileSystemDeviceConfigurationShare(objc.Ptr(c), objc.Ptr(share))
}

// SharedDirectory is a shared directory.
type SharedDirectory struct {
	*pointer
}

// NewSharedDirectory creates a new shared directory.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewSharedDirectory(dirPath string, readOnly bool) (*SharedDirectory, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dirPath); err != nil {
		return nil, err
	}

	dirPathChar := charWithGoString(dirPath)
	defer dirPathChar.Free()
	sd := &SharedDirectory{
		pointer: objc.NewPointer(
			C.newVZSharedDirectory(dirPathChar.CString(), C.bool(readOnly)),
		),
	}
	objc.SetFinalizer(sd, func(self *SharedDirectory) {
		objc.Release(self)
	})
	return sd, nil
}

// DirectoryShare is the base interface for a directory share.
type DirectoryShare interface {
	objc.NSObject

	directoryShare()
}

type baseDirectoryShare struct{}

func (*baseDirectoryShare) directoryShare() {}

var _ DirectoryShare = (*SingleDirectoryShare)(nil)

// SingleDirectoryShare defines the directory share for a single directory.
type SingleDirectoryShare struct {
	*pointer

	*baseDirectoryShare
}

// NewSingleDirectoryShare creates a new single directory share.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewSingleDirectoryShare(share *SharedDirectory) (*SingleDirectoryShare, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &SingleDirectoryShare{
		pointer: objc.NewPointer(
			C.newVZSingleDirectoryShare(objc.Ptr(share)),
		),
	}
	objc.SetFinalizer(config, func(self *SingleDirectoryShare) {
		objc.Release(self)
	})
	return config, nil
}

// MultipleDirectoryShare defines the directory share for multiple directories.
type MultipleDirectoryShare struct {
	*pointer

	*baseDirectoryShare
}

var _ DirectoryShare = (*MultipleDirectoryShare)(nil)

// NewMultipleDirectoryShare creates a new multiple directories share.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMultipleDirectoryShare(shares map[string]*SharedDirectory) (*MultipleDirectoryShare, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	directories := make(map[string]objc.NSObject, len(shares))
	for k, v := range shares {
		directories[k] = v
	}

	dict := objc.ConvertToNSMutableDictionary(directories)

	config := &MultipleDirectoryShare{
		pointer: objc.NewPointer(
			C.newVZMultipleDirectoryShare(objc.Ptr(dict)),
		),
	}
	objc.SetFinalizer(config, func(self *MultipleDirectoryShare) {
		objc.Release(self)
	})
	return config, nil
}

// MacOSGuestAutomountTag returns the macOS automount tag.
//
// A device configured with this tag will be automatically mounted in a macOS guest.
// This is only supported on macOS 13 and newer, error will be returned on older versions.
func MacOSGuestAutomountTag() (string, error) {
	if err := macOSAvailable(13); err != nil {
		return "", err
	}
	cstring := (*char)(C.getMacOSGuestAutomountTag())
	return cstring.String(), nil
}

// VirtioFileSystemDevice is a runtime Virtio file system device obtained from a running
// VirtualMachine via DirectorySharingDevices. Unlike VirtioFileSystemDeviceConfiguration
// (a configuration object set before start), this is the live device: its directory share
// can be swapped while the VM runs (macOS 12+), so the host can add or remove the
// directories a guest sees without recreating the VM.
//
// Don't create a VirtioFileSystemDevice directly. Request a file system device in your
// configuration and obtain the running instance via VirtualMachine.DirectorySharingDevices.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofilesystemdevice?language=objc
type VirtioFileSystemDevice struct {
	dispatchQueue unsafe.Pointer
	*pointer
}

func newVirtioFileSystemDevice(ptr, dispatchQueue unsafe.Pointer) *VirtioFileSystemDevice {
	return &VirtioFileSystemDevice{
		dispatchQueue: dispatchQueue,
		pointer:       objc.NewPointer(ptr),
	}
}

// SetShare swaps the directory share on this running device. The mutation runs on the
// VM's serial dispatch queue, as the framework requires for every VZVirtualMachine call.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofilesystemdevice/share?language=objc
func (d *VirtioFileSystemDevice) SetShare(share DirectoryShare) {
	C.setShareVZVirtioFileSystemDevice(objc.Ptr(d), objc.Ptr(share), d.dispatchQueue)
}

// DirectorySharingDevices returns the list of directory sharing devices configured on this
// running virtual machine, or an empty slice if none is configured. Since vz only creates
// VirtioFileSystemDevices, every element is one.
//
// This is only supported on macOS 12 and newer; nil is returned on older versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine/directorysharingdevices?language=objc
func (v *VirtualMachine) DirectorySharingDevices() []*VirtioFileSystemDevice {
	if err := macOSAvailable(12); err != nil {
		return nil
	}
	nsArray := objc.NewNSArray(
		C.VZVirtualMachine_directorySharingDevices(objc.Ptr(v)),
	)
	ptrs := nsArray.ToPointerSlice()
	devices := make([]*VirtioFileSystemDevice, len(ptrs))
	for i, ptr := range ptrs {
		devices[i] = newVirtioFileSystemDevice(ptr, v.dispatchQueue)
	}
	return devices
}
