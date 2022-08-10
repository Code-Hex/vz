package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

// DirectorySharingDeviceConfiguration for a directory sharing device configuration.
type DirectorySharingDeviceConfiguration interface {
	NSObject

	directorySharingDeviceConfiguration()
}

type baseDirectorySharingDeviceConfiguration struct{}

func (*baseDirectorySharingDeviceConfiguration) directorySharingDeviceConfiguration() {}

var _ DirectorySharingDeviceConfiguration = (*VirtioFileSystemDeviceConfiguration)(nil)

// VirtioFileSystemDeviceConfiguration is a configuration of a Virtio file system device.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofilesystemdeviceconfiguration?language=objc
type VirtioFileSystemDeviceConfiguration struct {
	pointer

	*baseDirectorySharingDeviceConfiguration
}

// NewVirtioFileSystemDeviceConfiguration create a new VirtioFileSystemDeviceConfiguration.
func NewVirtioFileSystemDeviceConfiguration(tag string) *VirtioFileSystemDeviceConfiguration {
	tagChar := charWithGoString(tag)
	defer tagChar.Free()
	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioFileSystemDeviceConfiguration(tagChar.CString()),
		},
	}
	runtime.SetFinalizer(fsdConfig, func(self *VirtioFileSystemDeviceConfiguration) {
		self.Release()
	})
	return fsdConfig
}

// SetDirectoryShare sets the directory share associated with this configuration.
func (c *VirtioFileSystemDeviceConfiguration) SetDirectoryShare(share DirectoryShare) {
	C.setVZVirtioFileSystemDeviceConfigurationShare(c.Ptr(), share.Ptr())
}

// SharedDirectory is a shared directory.
type SharedDirectory struct {
	pointer
}

// NewSharedDirectory creates a new shared directory.
func NewSharedDirectory(dirPath string, readOnly bool) *SharedDirectory {
	dirPathChar := charWithGoString(dirPath)
	defer dirPathChar.Free()
	sd := &SharedDirectory{
		pointer: pointer{
			ptr: C.newVZSharedDirectory(dirPathChar.CString(), C.bool(readOnly)),
		},
	}
	runtime.SetFinalizer(sd, func(self *SharedDirectory) {
		self.Release()
	})
	return sd
}

// DirectoryShare is the base interface for a directory share.
type DirectoryShare interface {
	NSObject

	directoryShare()
}

type baseDirectoryShare struct{}

func (*baseDirectoryShare) directoryShare() {}

var _ DirectoryShare = (*SingleDirectoryShare)(nil)

// SingleDirectoryShare defines the directory share for a single directory.
type SingleDirectoryShare struct {
	pointer

	*baseDirectoryShare
}

// NewSingleDirectoryShare creates a new single directory share.
func NewSingleDirectoryShare(share *SharedDirectory) *SingleDirectoryShare {
	config := &SingleDirectoryShare{
		pointer: pointer{
			ptr: C.newVZSingleDirectoryShare(share.Ptr()),
		},
	}
	runtime.SetFinalizer(config, func(self *SingleDirectoryShare) {
		self.Release()
	})
	return config
}

// MultipleDirectoryShare defines the directory share for multiple directories.
type MultipleDirectoryShare struct {
	pointer

	*baseDirectoryShare
}

// NewMultipleDirectoryShare creates a new multiple directories share.
func NewMultipleDirectoryShare(shares map[string]*SharedDirectory) *MultipleDirectoryShare {
	directories := make(map[string]NSObject, len(shares))
	for k, v := range shares {
		directories[k] = v
	}

	dict := convertToNSMutableDictionary(directories)

	config := &MultipleDirectoryShare{
		pointer: pointer{
			ptr: C.newVZMultipleDirectoryShare(dict.Ptr()),
		},
	}
	runtime.SetFinalizer(config, func(self *SingleDirectoryShare) {
		self.Release()
	})
	return config
}
