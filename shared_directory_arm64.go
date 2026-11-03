//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// LinuxRosettaAvailability represents an availability of Rosetta support for Linux binaries.
//
//go:generate go run ./cmd/addtags -tags=darwin,arm64 -file linuxrosettaavailability_string_arm64.go stringer -type=LinuxRosettaAvailability -output=linuxrosettaavailability_string_arm64.go
type LinuxRosettaAvailability int

const (
	// LinuxRosettaAvailabilityNotSupported Rosetta support for Linux binaries is not available on the host system.
	LinuxRosettaAvailabilityNotSupported LinuxRosettaAvailability = iota

	// LinuxRosettaAvailabilityNotInstalled Rosetta support for Linux binaries is not installed on the host system.
	LinuxRosettaAvailabilityNotInstalled

	// LinuxRosettaAvailabilityInstalled Rosetta support for Linux is installed on the host system.
	LinuxRosettaAvailabilityInstalled
)

// LinuxRosettaDirectoryShare directory share to enable Rosetta support for Linux binaries.
// see: https://developer.apple.com/documentation/virtualization/vzlinuxrosettadirectoryshare?language=objc
type LinuxRosettaDirectoryShare struct {
	*pointer

	*baseDirectoryShare
}

var _ DirectoryShare = (*LinuxRosettaDirectoryShare)(nil)

// NewLinuxRosettaDirectoryShare creates a new Rosetta directory share if Rosetta support
// for Linux binaries is installed.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewLinuxRosettaDirectoryShare() (*LinuxRosettaDirectoryShare, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}
	var nserrPtr unsafe.Pointer
	ds := &LinuxRosettaDirectoryShare{
		pointer: vzbridge.VZLinuxRosettaDirectoryShare_InitWithError(&nserrPtr),
	}
	if err := objcutil.NewNSError(nserrPtr); err != nil {
		return nil, err
	}
	return ds, nil
}

// SetOptions enables translation caching and configure the socket communication type for Rosetta.
//
// This is only supported on macOS 14 and newer. Older versions do nothing.
func (ds *LinuxRosettaDirectoryShare) SetOptions(options LinuxRosettaCachingOptions) {
	if err := macOSAvailable(14); err != nil {
		return
	}
	vzbridge.VZLinuxRosettaDirectoryShare_SetOptions(ds, options)
}

// LinuxRosettaDirectoryShareInstallRosetta download and install Rosetta support
// for Linux binaries if necessary.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func LinuxRosettaDirectoryShareInstallRosetta() error {
	if err := macOSAvailable(13); err != nil {
		return err
	}
	errCh := make(chan error, 1)
	request := registerNativeRequest(func(kind uint32, first, second unsafe.Pointer, value uint64) uint64 {
		if first == nil {
			errCh <- nil
		} else {
			errCh <- objcutil.NewNSError(first)
		}
		return 0
	})
	vzbridge.LinuxInstallRosetta(request)
	return <-errCh
}

// LinuxRosettaDirectoryShareAvailability checks the availability of Rosetta support
// for the directory share.
//
// This is only supported on macOS 13 and newer, LinuxRosettaAvailabilityNotSupported will
// be returned on older versions.
func LinuxRosettaDirectoryShareAvailability() LinuxRosettaAvailability {
	if err := macOSAvailable(13); err != nil {
		return LinuxRosettaAvailabilityNotSupported
	}
	return LinuxRosettaAvailability(vzbridge.VZLinuxRosettaDirectoryShare_Availability())
}

// LinuxRosettaCachingOptions for a directory sharing device configuration.
type LinuxRosettaCachingOptions interface {
	objc.NSObject

	linuxRosettaCachingOptions()
}

type baseLinuxRosettaCachingOptions struct{}

func (*baseLinuxRosettaCachingOptions) linuxRosettaCachingOptions() {}

// LinuxRosettaUnixSocketCachingOptions is an struct that represents caching options
// for a UNIX domain socket.
//
// This struct configures Rosetta to communicate with the Rosetta daemon using a UNIX domain socket.
type LinuxRosettaUnixSocketCachingOptions struct {
	*pointer

	*baseLinuxRosettaCachingOptions
}

var _ LinuxRosettaCachingOptions = (*LinuxRosettaUnixSocketCachingOptions)(nil)

// NewLinuxRosettaUnixSocketCachingOptions creates caching options for the Rosetta
// translation daemon's Unix domain socket at path.
//
// The path must be valid UTF-8 and contain no NUL bytes.
//
// This function requires macOS 14 or later. It returns an error on earlier versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzlinuxrosettaunixsocketcachingoptions?language=objc
func NewLinuxRosettaUnixSocketCachingOptions(path string) (*LinuxRosettaUnixSocketCachingOptions, error) {
	if err := macOSAvailable(14); err != nil {
		return nil, err
	}
	if !validUTF8CString(path) {
		return nil, fmt.Errorf("path must be valid UTF-8 without NUL bytes")
	}
	maxPathLen := vzbridge.VZLinuxRosettaUnixSocketCachingOptions_MaximumPathLength()
	if int(maxPathLen) < len(path) {
		return nil, fmt.Errorf("path length exceeds maximum allowed length of %d", maxPathLen)
	}

	var nserrPtr unsafe.Pointer
	usco := &LinuxRosettaUnixSocketCachingOptions{
		pointer: vzbridge.VZLinuxRosettaUnixSocketCachingOptions_InitWithPath_Error(vzbridge.NSString_StringWithUTF8String(path), &nserrPtr),
	}
	if err := objcutil.NewNSError(nserrPtr); err != nil {
		return nil, err
	}
	return usco, nil
}

// LinuxRosettaAbstractSocketCachingOptions is caching options for an abstract socket.
//
// Use this object to configure Rosetta to communicate with the Rosetta daemon using an abstract socket.
type LinuxRosettaAbstractSocketCachingOptions struct {
	*pointer

	*baseLinuxRosettaCachingOptions
}

var _ LinuxRosettaCachingOptions = (*LinuxRosettaAbstractSocketCachingOptions)(nil)

// NewLinuxRosettaAbstractSocketCachingOptions creates caching options for the
// Rosetta translation daemon's abstract socket with the given name.
//
// The name must be valid UTF-8 and contain no NUL bytes.
//
// This function requires macOS 14 or later. It returns an error on earlier versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzlinuxrosettaabstractsocketcachingoptions?language=objc
func NewLinuxRosettaAbstractSocketCachingOptions(name string) (*LinuxRosettaAbstractSocketCachingOptions, error) {
	if err := macOSAvailable(14); err != nil {
		return nil, err
	}
	if !validUTF8CString(name) {
		return nil, fmt.Errorf("name must be valid UTF-8 without NUL bytes")
	}
	maxNameLen := vzbridge.VZLinuxRosettaAbstractSocketCachingOptions_MaximumNameLength()
	if int(maxNameLen) < len(name) {
		return nil, fmt.Errorf("name length exceeds maximum allowed length of %d", maxNameLen)
	}

	var nserrPtr unsafe.Pointer
	asco := &LinuxRosettaAbstractSocketCachingOptions{
		pointer: vzbridge.VZLinuxRosettaAbstractSocketCachingOptions_InitWithName_Error(vzbridge.NSString_StringWithUTF8String(name), &nserrPtr),
	}
	if err := objcutil.NewNSError(nserrPtr); err != nil {
		return nil, err
	}
	return asco, nil
}
