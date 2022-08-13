package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

type PointingDeviceConfiguration interface {
	NSObject

	pointingDeviceConfiguration()
}

type basePointingDeviceConfiguration struct{}

func (*basePointingDeviceConfiguration) pointingDeviceConfiguration() {}

type USBScreenCoordinatePointingDeviceConfiguration struct {
	pointer

	*basePointingDeviceConfiguration
}

var _ PointingDeviceConfiguration = (*USBScreenCoordinatePointingDeviceConfiguration)(nil)

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
