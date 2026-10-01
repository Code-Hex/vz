//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

// MacGraphicsDeviceConfiguration is a configuration for a display attached to a Mac graphics device.
type MacGraphicsDeviceConfiguration struct {
	*pointer

	*baseGraphicsDeviceConfiguration
}

var _ GraphicsDeviceConfiguration = (*MacGraphicsDeviceConfiguration)(nil)

// NewMacGraphicsDeviceConfiguration creates a new MacGraphicsDeviceConfiguration.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacGraphicsDeviceConfiguration() (*MacGraphicsDeviceConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	graphicsConfiguration := &MacGraphicsDeviceConfiguration{
		pointer: vzbridge.VZMacGraphicsDeviceConfiguration_Init(),
	}
	return graphicsConfiguration, nil
}

// SetDisplays sets the displays associated with this graphics device.
func (m *MacGraphicsDeviceConfiguration) SetDisplays(displayConfigs ...*MacGraphicsDisplayConfiguration) {
	array := nativeObjectArray(displayConfigs)
	vzbridge.VZMacGraphicsDeviceConfiguration_SetDisplays(m, array)
}

// MacGraphicsDisplayConfiguration is the configuration for a Mac graphics device.
type MacGraphicsDisplayConfiguration struct {
	*pointer
}

// NewMacGraphicsDisplayConfiguration creates a new MacGraphicsDisplayConfiguration.
//
// Creates a display configuration with the specified pixel dimensions and pixel density.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacGraphicsDisplayConfiguration(widthInPixels int64, heightInPixels int64, pixelsPerInch int64) (*MacGraphicsDisplayConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	graphicsDisplayConfiguration := &MacGraphicsDisplayConfiguration{
		pointer: vzbridge.VZMacGraphicsDisplayConfiguration_InitWithWidthInPixels_HeightInPixels_PixelsPerInch(int64(widthInPixels), int64(heightInPixels), int64(pixelsPerInch)),
	}
	return graphicsDisplayConfiguration, nil
}
