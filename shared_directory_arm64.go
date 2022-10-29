//go:build darwin && arm64
// +build darwin,arm64

package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_13_arm64.h"
*/
import "C"
import (
	"runtime"
	"runtime/cgo"
	"unsafe"
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

//export linuxInstallRosettaWithCompletionHandler
func linuxInstallRosettaWithCompletionHandler(cgoHandlerPtr, errPtr unsafe.Pointer) {
	cgoHandler := *(*cgo.Handle)(cgoHandlerPtr)

	handler := cgoHandler.Value().(func(error))

	if err := newNSError(errPtr); err != nil {
		handler(err)
	} else {
		handler(nil)
	}
}

// LinuxRosettaDirectoryShare directory share to enable Rosetta support for Linux binaries.
// see: https://developer.apple.com/documentation/virtualization/vzlinuxrosettadirectoryshare?language=objc
type LinuxRosettaDirectoryShare struct {
	pointer

	*baseDirectoryShare
}

var _ DirectoryShare = (*LinuxRosettaDirectoryShare)(nil)

// NewLinuxRosettaDirectoryShare creates a new Rosetta directory share if Rosetta support
// for Linux binaries is installed.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewLinuxRosettaDirectoryShare() (*LinuxRosettaDirectoryShare, error) {
	if macosMajorVersionLessThan(13) {
		return nil, ErrUnsupportedOSVersion
	}
	nserr := newNSErrorAsNil()
	nserrPtr := nserr.Ptr()
	ds := &LinuxRosettaDirectoryShare{
		pointer: pointer{
			ptr: C.newVZLinuxRosettaDirectoryShare(&nserrPtr),
		},
	}
	if err := newNSError(nserrPtr); err != nil {
		return nil, err
	}
	runtime.SetFinalizer(ds, func(self *LinuxRosettaDirectoryShare) {
		self.Release()
	})
	return ds, nil
}

// LinuxRosettaDirectoryShareInstallRosetta download and install Rosetta support
// for Linux binaries if necessary.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func LinuxRosettaDirectoryShareInstallRosetta() error {
	if macosMajorVersionLessThan(13) {
		return ErrUnsupportedOSVersion
	}
	errCh := make(chan error, 1)
	cgoHandler := cgo.NewHandle(func(err error) {
		errCh <- err
	})
	C.linuxInstallRosetta(unsafe.Pointer(&cgoHandler))
	return <-errCh
}

// LinuxRosettaDirectoryShareAvailability checks the availability of Rosetta support
// for the directory share.
//
// This is only supported on macOS 13 and newer, LinuxRosettaAvailabilityNotSupported will
// be returned on older versions.
func LinuxRosettaDirectoryShareAvailability() LinuxRosettaAvailability {
	if macosMajorVersionLessThan(13) {
		return LinuxRosettaAvailabilityNotSupported
	}
	return LinuxRosettaAvailability(C.availabilityVZLinuxRosettaDirectoryShare())
}
