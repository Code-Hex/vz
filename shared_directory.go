package vz

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
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
	if err := objcutil.NewNSError(nserrPtr); err != nil {
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
// It returns an error if share is nil or uninitialized.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewSingleDirectoryShare(share *SharedDirectory) (*SingleDirectoryShare, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	if share == nil || objc.Ptr(share) == nil {
		return nil, fmt.Errorf("shared directory must not be nil or uninitialized")
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

// CanonicalizeDirectoryShareName removes *, /, :, <, >, ?, \, and | from name
// so it can be used as a key in NewMultipleDirectoryShare.
// It discards the first NUL byte and everything after it.
// It returns an error if name contains invalid UTF-8, or if the
// result is empty, is "." or "..", or exceeds 255 bytes in UTF-8.
// Other characters, including whitespace and combining characters, are preserved.
// Different inputs can produce the same name; callers must handle collisions
// before adding names to a share map.
//
// This function runs entirely in Go and does not require macOS 12.
//
// see: https://developer.apple.com/documentation/virtualization/vzmultipledirectoryshare/canonicalizedname(from:)?language=objc
func CanonicalizeDirectoryShareName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("directory share name must be valid UTF-8")
	}
	name, _, _ = strings.Cut(name, "\x00")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '*', '/', ':', '<', '>', '?', '\\', '|':
			return -1
		}
		return r
	}, name)
	switch name {
	case "", ".", "..":
		return "", fmt.Errorf("cannot produce a valid directory share name")
	}
	if len(name) > 255 {
		return "", fmt.Errorf("directory share name exceeds 255 UTF-8 bytes")
	}
	return name, nil
}

// NewMultipleDirectoryShare creates a new multiple directories share.
// It returns an error if a directory name is invalid or a directory is nil or uninitialized.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMultipleDirectoryShare(shares map[string]*SharedDirectory) (*MultipleDirectoryShare, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	directories := make(map[string]objc.NSObject, len(shares))
	for name, directory := range shares {
		if directory == nil || objc.Ptr(directory) == nil {
			return nil, fmt.Errorf("directory share %q must not be nil or uninitialized", name)
		}
		if !validUTF8CString(name) {
			return nil, fmt.Errorf("invalid directory share name %q: must be valid UTF-8 without NUL bytes", name)
		}
		nativeName := vzbridge.NSString_StringWithUTF8String(name)
		var nserrPtr unsafe.Pointer
		valid := vzbridge.VZMultipleDirectoryShare_ValidateName_Error(nativeName, &nserrPtr)
		objc.Release(nativeName)
		if !valid {
			return nil, fmt.Errorf("invalid directory share name %q: %w", name, objcutil.NewNSError(nserrPtr))
		}
		directories[name] = directory
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
	return objcutil.String(vzbridge.VZVirtioFileSystemDeviceConfiguration_MacOSGuestAutomountTag()), nil
}
