//go:build darwin && arm64

package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// MacGuestProvisioningOptions configures an account on a macOS guest's first boot
// after restore. The guest must run macOS 27 or later. These options cannot
// reconfigure a guest that has already been provisioned.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions?language=objc
type MacGuestProvisioningOptions struct {
	*pointer
}

// MacGuestProvisioningOption configures initial macOS guest setup.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions?language=objc
type MacGuestProvisioningOption func(*MacGuestProvisioningOptions)

// WithGuestAutomaticLogin controls automatic login at startup.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/logsinautomatically?language=objc
func WithGuestAutomaticLogin(enabled bool) MacGuestProvisioningOption {
	return func(options *MacGuestProvisioningOptions) {
		vzbridge.VZMacGuestProvisioningOptions_SetLogsInAutomatically(options, enabled)
	}
}

// WithGuestRemoteLogin controls SSH access to the guest. It is disabled by default.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/enablesremotelogin?language=objc
func WithGuestRemoteLogin(enabled bool) MacGuestProvisioningOption {
	return func(options *MacGuestProvisioningOptions) {
		vzbridge.VZMacGuestProvisioningOptions_SetEnablesRemoteLogin(options, enabled)
	}
}

// NewMacGuestProvisioningOptions creates initial account settings on macOS 27 or later.
// The framework validates these settings when WithGuestProvisioningOptions is applied.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/init()?language=objc
func NewMacGuestProvisioningOptions(fullName, username, password string, opts ...MacGuestProvisioningOption) (*MacGuestProvisioningOptions, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	for _, value := range []string{fullName, username, password} {
		if !validUTF8CString(value) {
			return nil, fmt.Errorf("guest account settings must be valid UTF-8 without NUL bytes")
		}
	}
	options := &MacGuestProvisioningOptions{
		pointer: vzbridge.VZMacGuestProvisioningOptions_Init(),
	}
	name := vzbridge.NSString_StringWithUTF8String(fullName)
	defer objc.Release(name)
	user := vzbridge.NSString_StringWithUTF8String(username)
	defer objc.Release(user)
	secret := vzbridge.NSString_StringWithUTF8String(password)
	defer objc.Release(secret)
	vzbridge.VZMacGuestProvisioningOptions_SetFullName(options, name)
	vzbridge.VZMacGuestProvisioningOptions_SetUsername(options, user)
	vzbridge.VZMacGuestProvisioningOptions_SetPassword(options, secret)
	for _, opt := range opts {
		opt(options)
	}
	return options, nil
}

// WithGuestProvisioningOptions applies initial macOS guest setup on macOS 27 or later.
// The guest evaluates these options only on its first boot after restore.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacosvirtualmachinestartoptions/setguestprovisioning(_:)?language=objc
func WithGuestProvisioningOptions(options *MacGuestProvisioningOptions) VirtualMachineStartOption {
	return func(start *virtualMachineStartOptions) error {
		if err := macOSAvailable(27); err != nil {
			return err
		}
		if options == nil || objc.Ptr(options) == nil {
			return fmt.Errorf("guest provisioning options must not be nil")
		}
		if start.macOSVirtualMachineStartOptionsPtr == nil {
			start.macOSVirtualMachineStartOptionsPtr = vzbridge.VZMacOSVirtualMachineStartOptions_New()
		}
		var nativeError unsafe.Pointer
		vzbridge.VZMacOSVirtualMachineStartOptions_SetGuestProvisioningOptions_Error(start.macOSVirtualMachineStartOptionsPtr, options, &nativeError)
		if err := objcutil.NewNSError(nativeError); err != nil {
			return err
		}
		return nil
	}
}

// FullName returns the full name of the initial guest account.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/fullname?language=objc
func (o *MacGuestProvisioningOptions) FullName() string {
	return objcutil.String(vzbridge.VZMacGuestProvisioningOptions_FullName(o))
}

// Username returns the login name of the initial guest account.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/username?language=objc
func (o *MacGuestProvisioningOptions) Username() string {
	return objcutil.String(vzbridge.VZMacGuestProvisioningOptions_Username(o))
}

// Password returns the password of the initial guest account.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/password?language=objc
func (o *MacGuestProvisioningOptions) Password() string {
	return objcutil.String(vzbridge.VZMacGuestProvisioningOptions_Password(o))
}

// LogsInAutomatically reports whether the guest logs in automatically.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/logsinautomatically?language=objc
func (o *MacGuestProvisioningOptions) LogsInAutomatically() bool {
	return vzbridge.VZMacGuestProvisioningOptions_LogsInAutomatically(o)
}

// EnablesRemoteLogin reports whether the guest enables SSH access.
//
// see: https://developer.apple.com/documentation/virtualization/vzmacguestprovisioningoptions/enablesremotelogin?language=objc
func (o *MacGuestProvisioningOptions) EnablesRemoteLogin() bool {
	return vzbridge.VZMacGuestProvisioningOptions_EnablesRemoteLogin(o)
}
