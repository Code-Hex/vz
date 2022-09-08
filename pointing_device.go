package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

// PointingDeviceConfiguration is an interface for a pointing device configuration.
type PointingDeviceConfiguration interface {
	NSObject

	pointingDeviceConfiguration()
}

type basePointingDeviceConfiguration struct{}

func (*basePointingDeviceConfiguration) pointingDeviceConfiguration() {}

// USBScreenCoordinatePointingDeviceConfiguration is a struct that defines the configuration
// for a USB pointing device that reports absolute coordinates.
type USBScreenCoordinatePointingDeviceConfiguration struct {
	pointer

	*basePointingDeviceConfiguration
}

var _ PointingDeviceConfiguration = (*USBScreenCoordinatePointingDeviceConfiguration)(nil)

// NewUSBScreenCoordinatePointingDeviceConfiguration creates a new USBScreenCoordinatePointingDeviceConfiguration.
//
// This is only supported on macOS 12 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewUSBScreenCoordinatePointingDeviceConfiguration() (*USBScreenCoordinatePointingDeviceConfiguration, error) {
	if macosMajorVersionLessThan(12) {
		return nil, ErrUnsupportedOSVersion
	}
	config := &USBScreenCoordinatePointingDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZUSBScreenCoordinatePointingDeviceConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *USBScreenCoordinatePointingDeviceConfiguration) {
		self.Release()
	})
	return config, nil
}
