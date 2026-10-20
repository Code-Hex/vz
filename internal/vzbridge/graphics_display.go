//go:build darwin

package vzbridge

import "github.com/Code-Hex/vz/v4/internal/objc"

func MacGraphicsDisplayPixelsPerInch(display objc.NSObject) (int64, bool) {
	if !NSObject_IsKindOfClass(display, directGetClass("VZMacGraphicsDisplay")) {
		return 0, false
	}
	return VZMacGraphicsDisplay_PixelsPerInch(display), true
}
