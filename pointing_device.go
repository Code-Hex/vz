//go:build darwin

package vz

import (
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// PointingDeviceConfiguration is an interface for a pointing device configuration.
type PointingDeviceConfiguration interface {
	objc.NSObject

	pointingDeviceConfiguration()
}

type basePointingDeviceConfiguration struct{}

func (*basePointingDeviceConfiguration) pointingDeviceConfiguration() {}

// USBScreenCoordinatePointingDeviceConfiguration is a struct that defines the configuration
// for a USB pointing device that reports absolute coordinates.
type USBScreenCoordinatePointingDeviceConfiguration struct {
	*pointer

	*basePointingDeviceConfiguration
}

var _ PointingDeviceConfiguration = (*USBScreenCoordinatePointingDeviceConfiguration)(nil)

// NewUSBScreenCoordinatePointingDeviceConfiguration creates a new USBScreenCoordinatePointingDeviceConfiguration.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewUSBScreenCoordinatePointingDeviceConfiguration() (*USBScreenCoordinatePointingDeviceConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &USBScreenCoordinatePointingDeviceConfiguration{
		pointer: vzbridge.VZUSBScreenCoordinatePointingDeviceConfiguration_Init(),
	}
	return config, nil
}
