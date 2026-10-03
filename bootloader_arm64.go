//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// MacOSBootLoader is a boot loader configuration for booting macOS on Apple Silicon.
type MacOSBootLoader struct {
	*pointer

	*baseBootLoader
}

var _ BootLoader = (*MacOSBootLoader)(nil)

// NewMacOSBootLoader creates a new MacOSBootLoader struct.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacOSBootLoader() (*MacOSBootLoader, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	bootLoader := &MacOSBootLoader{
		pointer: vzbridge.VZMacOSBootLoader_Init(),
	}
	return bootLoader, nil
}
