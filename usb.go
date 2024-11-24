package vz

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_15.h"
*/
import "C"
import (
	"github.com/Code-Hex/vz/v3/internal/objc"
)

// USBControllerConfiguration for a usb controller configuration.
type USBControllerConfiguration interface {
	objc.NSObject

	usbControllerConfiguration()
}

type baseUSBControllerConfiguration struct{}

func (*baseUSBControllerConfiguration) usbControllerConfiguration() {}

// XHCIControllerConfiguration is a configuration of the USB XHCI controller.
//
// This configuration creates a This configuration creates a USB XHCI controller device for the guest.
// see: https://developer.apple.com/documentation/virtualization/vzxhcicontrollerconfiguration?language=objc
type XHCIControllerConfiguration struct {
	*pointer

	*baseUSBControllerConfiguration
}

var _ USBControllerConfiguration = (*XHCIControllerConfiguration)(nil)

// NewXHCIControllerConfiguration creates a new XHCIControllerConfiguration.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
func NewXHCIControllerConfiguration() (*XHCIControllerConfiguration, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}

	config := &XHCIControllerConfiguration{
		pointer: objc.NewPointer(C.newVZXHCIControllerConfiguration()),
	}

	objc.SetFinalizer(config, func(self *XHCIControllerConfiguration) {
		objc.Release(self)
	})
	return config, nil
}
