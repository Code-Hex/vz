package vz

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// VirtualMachineConfigurationOption configures a virtual machine before creation.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration?language=objc
type VirtualMachineConfigurationOption func(*VirtualMachineConfiguration) error

// WithVirtualMachineLabel sets the name used for the VM in some system services.
// The label must contain 1 to 64 characters, including a non-whitespace character.
// It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration/label?language=objc
func WithVirtualMachineLabel(label string) VirtualMachineConfigurationOption {
	return func(config *VirtualMachineConfiguration) error {
		if err := macOSAvailable(27); err != nil {
			return err
		}
		if !validUTF8CString(label) {
			return fmt.Errorf("VM label must be valid UTF-8 without NUL bytes")
		}
		if utf8.RuneCountInString(label) > 64 || strings.TrimSpace(label) == "" {
			return fmt.Errorf("VM label must contain 1 to 64 valid characters, including non-whitespace")
		}
		value := vzbridge.NSString_StringWithUTF8String(label)
		defer objc.Release(value)
		vzbridge.VZVirtualMachineConfiguration_SetLabel(config, value)
		return nil
	}
}

// Label returns the name used for the VM in system services, or an empty string
// if no label was set. It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration/label?language=objc
func (v *VirtualMachineConfiguration) Label() (string, error) {
	if err := macOSAvailable(27); err != nil {
		return "", err
	}
	return objcutil.String(vzbridge.VZVirtualMachineConfiguration_Label(v)), nil
}
