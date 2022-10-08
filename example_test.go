package vz_test

import (
	"errors"

	"github.com/Code-Hex/vz/v2"
)

func ExampleErrUnsupportedOSVersion() {
	// The vz.NewSharedDirectory API is only available on macOS 11 or newer
	_, err := vz.NewSharedDirectory("/example/path", false)
	if errors.Is(err, vz.ErrUnsupportedOSVersion) {
		// When running on macOS 10, an error will be returned when
		// trying to use NewSharedDirectory().
	}
	// When running on macOS 11 or newer, vz.NewSharedDirectory() can be
	// called without getting an ErrUnsupportedOSVersion error.
}
