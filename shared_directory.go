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

	"github.com/Code-Hex/vz/v2/internal/objc"
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
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewVirtioFileSystemDeviceConfiguration(tag string) (*VirtioFileSystemDeviceConfiguration, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
	tagChar := charWithGoString(tag)
	defer tagChar.Free()

	nserr := objc.NewNSErrorAsNil()
	nserrPtr := objc.Ptr(nserr)

	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: objc.NewPointer(
			C.newVZVirtioFileSystemDeviceConfiguration(tagChar.CString(), &nserrPtr),
		),
	}
	if err := objc.NewNSError(nserrPtr); err != nil {
		return nil, err
	}
	runtime.SetFinalizer(fsdConfig, func(self *VirtioFileSystemDeviceConfiguration) {
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
		pointer: objc.NewPointer(
			C.newVZSharedDirectory(dirPathChar.CString(), C.bool(readOnly)),
		),
	}
	runtime.SetFinalizer(sd, func(self *SharedDirectory) {
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
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewSingleDirectoryShare(share *SharedDirectory) (*SingleDirectoryShare, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
	config := &SingleDirectoryShare{
		pointer: objc.NewPointer(
			C.newVZSingleDirectoryShare(objc.Ptr(share)),
		),
	}
	runtime.SetFinalizer(config, func(self *SingleDirectoryShare) {
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
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewMultipleDirectoryShare(shares map[string]*SharedDirectory) (*MultipleDirectoryShare, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
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
	runtime.SetFinalizer(config, func(self *MultipleDirectoryShare) {
		objc.Release(self)
	})
	return config, nil
}

// MacOSGuestAutomountTag returns the macOS automount tag.
//
// A device configured with this tag will be automatically mounted in a macOS guest.
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func MacOSGuestAutomountTag() (string, error) {
	if macosMajorVersionLessThan(13) {
		return "", ErrUnsupportedOSVersion
	}
	cstring := (*char)(C.getMacOSGuestAutomountTag())
	return cstring.String(), nil
}
