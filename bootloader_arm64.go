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

// MacOSBootLoader is a boot loader configuration for booting macOS on Apple Silicon.
type MacOSBootLoader struct {
	pointer

	*baseBootLoader
}

var _ BootLoader = (*MacOSBootLoader)(nil)

// NewMacOSBootLoader creates a new MacOSBootLoader struct.
func NewMacOSBootLoader() *MacOSBootLoader {
	bootLoader := &MacOSBootLoader{
		pointer: pointer{
			ptr: C.newVZMacOSBootLoader(),
		},
	}
	runtime.SetFinalizer(bootLoader, func(self *MacOSBootLoader) {
		self.Release()
	})
	return bootLoader
}
