package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

type PlatformConfiguration interface {
	NSObject

	platformConfiguration()
}

type basePlatformConfiguration struct{}

func (*basePlatformConfiguration) platformConfiguration() {}

type GenericPlatformConfiguration struct {
	pointer

	*basePlatformConfiguration
}

var _ PlatformConfiguration = (*GenericPlatformConfiguration)(nil)

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
