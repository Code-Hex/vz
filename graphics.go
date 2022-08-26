package vz

// GraphicsDeviceConfiguration is an interface for a graphics device configuration.
type GraphicsDeviceConfiguration interface {
	NSObject

	graphicsDeviceConfiguration()
}

type baseGraphicsDeviceConfiguration struct{}

func (*baseGraphicsDeviceConfiguration) graphicsDeviceConfiguration() {}
