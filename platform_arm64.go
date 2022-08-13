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

type MacPlatformConfiguration struct {
	pointer

	*basePlatformConfiguration
}

var _ PlatformConfiguration = (*MacPlatformConfiguration)(nil)

func NewMacPlatformConfiguration() *MacPlatformConfiguration {
	platformConfig := &MacPlatformConfiguration{
		pointer: pointer{
			ptr: C.newVZMacPlatformConfiguration(),
		},
	}
	runtime.SetFinalizer(platformConfig, func(self *MacPlatformConfiguration) {
		self.Release()
	})
	return platformConfig
}
