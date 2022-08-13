package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

// KeyboardConfiguration interface for a keyboard configuration.
type KeyboardConfiguration interface {
	NSObject

	keyboardConfiguration()
}

type baseKeyboardConfiguration struct{}

func (*baseKeyboardConfiguration) keyboardConfiguration() {}

type USBKeyboardConfiguration struct {
	pointer

	*baseKeyboardConfiguration
}

var _ KeyboardConfiguration = (*USBKeyboardConfiguration)(nil)

func NewUSBKeyboardConfiguration() *USBKeyboardConfiguration {
	config := &USBKeyboardConfiguration{
		pointer: pointer{
			ptr: C.newVZUSBKeyboardConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *USBKeyboardConfiguration) {
		self.Release()
	})
	return config
}
