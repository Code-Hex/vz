//go:build darwin

package vz

import (
	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

// KeyboardConfiguration interface for a keyboard configuration.
type KeyboardConfiguration interface {
	objc.NSObject

	keyboardConfiguration()
}

type baseKeyboardConfiguration struct{}

func (*baseKeyboardConfiguration) keyboardConfiguration() {}

// USBKeyboardConfiguration is a device that defines the configuration for a USB keyboard.
type USBKeyboardConfiguration struct {
	*pointer

	*baseKeyboardConfiguration
}

var _ KeyboardConfiguration = (*USBKeyboardConfiguration)(nil)

// NewUSBKeyboardConfiguration creates a new USB keyboard configuration.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewUSBKeyboardConfiguration() (*USBKeyboardConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &USBKeyboardConfiguration{
		pointer: vzbridge.Framework_VZUSBKeyboardConfiguration_init_6e0fb328(),
	}
	return config, nil
}
