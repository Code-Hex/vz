//go:build darwin && arm64
// +build darwin,arm64

package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_13_arm64.h"
*/
import "C"
import "runtime"

// MacTrackpadConfiguration is a struct that defines the configuration
// for a Mac trackpad.
//
// This device is only recognized by virtual machines running macOS 13.0 and later.
// In order to support both macOS 13.0 and earlier guests, VirtualMachineConfiguration.pointingDevices
// can be set to an array containing both a MacTrackpadConfiguration and
// a USBScreenCoordinatePointingDeviceConfiguration object. macOS 13.0 and later guests will use
// the multi-touch trackpad device, while earlier versions of macOS will use the USB pointing device.
//
// see: https://developer.apple.com/documentation/virtualization/vzmactrackpadconfiguration?language=objc
type MacTrackpadConfiguration struct {
	pointer

	*basePointingDeviceConfiguration
}

var _ PointingDeviceConfiguration = (*MacTrackpadConfiguration)(nil)

// NewMacTrackpadConfiguration creates a new MacTrackpadConfiguration.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewMacTrackpadConfiguration() (*MacTrackpadConfiguration, error) {
	if macosMajorVersionLessThan(13) {
		return nil, ErrUnsupportedOSVersion
	}
	config := &MacTrackpadConfiguration{
		pointer: pointer{
			ptr: C.newVZMacTrackpadConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *MacTrackpadConfiguration) {
		self.Release()
	})
	return config, nil
}
