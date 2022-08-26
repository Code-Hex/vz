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
func NewUSBScreenCoordinatePointingDeviceConfiguration() *USBScreenCoordinatePointingDeviceConfiguration {
	config := &USBScreenCoordinatePointingDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZUSBScreenCoordinatePointingDeviceConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *USBScreenCoordinatePointingDeviceConfiguration) {
		self.Release()
	})
	return config
}
