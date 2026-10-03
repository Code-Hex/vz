//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// MacKeyboardConfiguration is a struct that defines the configuration
// for a Mac keyboard.
//
// This device is only recognized by virtual machines running macOS 14.0 and later.
// In order to support both macOS 13.0 and earlier guests, VirtualMachineConfiguration.keyboards
// can be set to an array containing both a MacKeyboardConfiguration and
// a USBKeyboardConfiguration object. macOS 14.0 and later guests will use the Mac keyboard device,
// while earlier versions of macOS will use the USB keyboard device.
//
// see: https://developer.apple.com/documentation/virtualization/vzmackeyboardconfiguration?language=objc
type MacKeyboardConfiguration struct {
	*pointer

	*baseKeyboardConfiguration
}

var _ KeyboardConfiguration = (*MacKeyboardConfiguration)(nil)

// NewMacKeyboardConfiguration creates a new MacKeyboardConfiguration.
//
// This is only supported on macOS 14 and newer, error will
// be returned on older versions.
func NewMacKeyboardConfiguration() (*MacKeyboardConfiguration, error) {
	if err := macOSAvailable(14); err != nil {
		return nil, err
	}
	config := &MacKeyboardConfiguration{
		pointer: vzbridge.VZMacKeyboardConfiguration_Init(),
	}
	return config, nil
}
