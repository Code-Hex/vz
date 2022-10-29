package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
# include "virtualization_13.h"
*/
import "C"
import (
	"os"
	"runtime"
)

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
//
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewVirtioFileSystemDeviceConfiguration(tag string) (*VirtioFileSystemDeviceConfiguration, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
	tagChar := charWithGoString(tag)
	defer tagChar.Free()

	nserr := newNSErrorAsNil()
	nserrPtr := nserr.Ptr()

	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioFileSystemDeviceConfiguration(tagChar.CString(), &nserrPtr),
		},
	}
	if err := newNSError(nserrPtr); err != nil {
		return nil, err
	}
	runtime.SetFinalizer(fsdConfig, func(self *VirtioFileSystemDeviceConfiguration) {
		self.Release()
	})
	return fsdConfig, nil
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
//
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewSharedDirectory(dirPath string, readOnly bool) (*SharedDirectory, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
	if _, err := os.Stat(dirPath); err != nil {
		return nil, err
	}

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
	return sd, nil
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
//
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewSingleDirectoryShare(share *SharedDirectory) (*SingleDirectoryShare, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
	config := &SingleDirectoryShare{
		pointer: pointer{
			ptr: C.newVZSingleDirectoryShare(share.Ptr()),
		},
	}
	runtime.SetFinalizer(config, func(self *SingleDirectoryShare) {
		self.Release()
	})
	return config, nil
}

// MultipleDirectoryShare defines the directory share for multiple directories.
type MultipleDirectoryShare struct {
	pointer

	*baseDirectoryShare
}

var _ DirectoryShare = (*MultipleDirectoryShare)(nil)

// NewMultipleDirectoryShare creates a new multiple directories share.
//
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewMultipleDirectoryShare(shares map[string]*SharedDirectory) (*MultipleDirectoryShare, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
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
	runtime.SetFinalizer(config, func(self *MultipleDirectoryShare) {
		self.Release()
	})
	return config, nil
}
