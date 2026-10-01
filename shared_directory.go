package vz

import (
	"os"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
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

	var nserrPtr unsafe.Pointer
	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: vzbridge.NewVZVirtioFileSystemDeviceConfiguration(tag, &nserrPtr),
	}
	if err := newNSError(nserrPtr); err != nil {
		return nil, err
	}
	return fsdConfig, nil
}

// SetDirectoryShare sets the directory share associated with this configuration.
func (c *VirtioFileSystemDeviceConfiguration) SetDirectoryShare(share DirectoryShare) {
	vzbridge.VZVirtioFileSystemDeviceConfiguration_SetShare(c, share)
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
	sd := &SharedDirectory{
		pointer: vzbridge.VZSharedDirectory_InitWithURL_ReadOnly(vzbridge.NSURL_FileURLWithPath(vzbridge.NSString_StringWithUTF8String(dirPath)), bool(readOnly)),
	}
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
		pointer: vzbridge.VZSingleDirectoryShare_InitWithDirectory(share),
	}
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

	dict := nativeObjectDictionary(directories)

	config := &MultipleDirectoryShare{
		pointer: vzbridge.VZMultipleDirectoryShare_InitWithDirectories(dict),
	}
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
	return nativeString(vzbridge.VZVirtioFileSystemDeviceConfiguration_MacOSGuestAutomountTag()), nil
}
