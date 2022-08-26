package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

// PlatformConfiguration is an interface for a platform configuration.
type PlatformConfiguration interface {
	NSObject

	platformConfiguration()
}

type basePlatformConfiguration struct{}

func (*basePlatformConfiguration) platformConfiguration() {}

// GenericPlatformConfiguration is the platform configuration for a generic Intel or ARM virtual machine.
type GenericPlatformConfiguration struct {
	pointer

	*basePlatformConfiguration
}

var _ PlatformConfiguration = (*GenericPlatformConfiguration)(nil)

// NewGenericPlatformConfiguration creates a new generic platform configuration.
func NewGenericPlatformConfiguration() *GenericPlatformConfiguration {
	platformConfig := &GenericPlatformConfiguration{
		pointer: pointer{
			ptr: C.newVZGenericPlatformConfiguration(),
		},
	}
	runtime.SetFinalizer(platformConfig, func(self *GenericPlatformConfiguration) {
		self.Release()
	})
	return platformConfig
}
