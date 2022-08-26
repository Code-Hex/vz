//go:build darwin && arm64
// +build darwin,arm64

package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_arm64.h"
*/
import "C"
import "runtime"

// MacGraphicsDeviceConfiguration is a configuration for a display attached to a Mac graphics device.
type MacGraphicsDeviceConfiguration struct {
	pointer

	*baseGraphicsDeviceConfiguration
}

var _ GraphicsDeviceConfiguration = (*MacGraphicsDeviceConfiguration)(nil)

// NewMacGraphicsDeviceConfiguration creates a new MacGraphicsDeviceConfiguration.
func NewMacGraphicsDeviceConfiguration() *MacGraphicsDeviceConfiguration {
	graphicsConfiguration := &MacGraphicsDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZMacGraphicsDeviceConfiguration(),
		},
	}
	runtime.SetFinalizer(graphicsConfiguration, func(self *MacGraphicsDeviceConfiguration) {
		self.Release()
	})
	return graphicsConfiguration
}

// SetDisplays sets the displays associated with this graphics device.
func (m *MacGraphicsDeviceConfiguration) SetDisplays(displayConfigs ...*MacGraphicsDisplayConfiguration) {
	ptrs := make([]NSObject, len(displayConfigs))
	for i, val := range displayConfigs {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setDisplaysVZMacGraphicsDeviceConfiguration(m.Ptr(), array.Ptr())
}

// MacGraphicsDisplayConfiguration is the configuration for a Mac graphics device.
type MacGraphicsDisplayConfiguration struct {
	pointer
}

// NewMacGraphicsDisplayConfiguration creates a new MacGraphicsDisplayConfiguration.
//
// Creates a display configuration with the specified pixel dimensions and pixel density.
func NewMacGraphicsDisplayConfiguration(widthInPixels int64, heightInPixels int64, pixelsPerInch int64) *MacGraphicsDisplayConfiguration {
	graphicsDisplayConfiguration := &MacGraphicsDisplayConfiguration{
		pointer: pointer{
			ptr: C.newVZMacGraphicsDisplayConfiguration(
				C.NSInteger(widthInPixels),
				C.NSInteger(heightInPixels),
				C.NSInteger(pixelsPerInch),
			),
		},
	}
	runtime.SetFinalizer(graphicsDisplayConfiguration, func(self *MacGraphicsDisplayConfiguration) {
		self.Release()
	})
	return graphicsDisplayConfiguration
}
