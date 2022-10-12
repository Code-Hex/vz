//go:build darwin && debug
// +build darwin,debug

package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_debug.h"
*/
import "C"
import "runtime"

// DebugStubConfiguration is an interface to debug configuration.
type DebugStubConfiguration interface {
	NSObject

	debugStubConfiguration()
}

type baseDebugStubConfiguration struct{}

func (*baseDebugStubConfiguration) debugStubConfiguration() {}

// GDBDebugStubConfiguration is a configuration for gdb debugging.
type GDBDebugStubConfiguration struct {
	pointer

	*baseDebugStubConfiguration
}

var _ DebugStubConfiguration = (*GDBDebugStubConfiguration)(nil)

// NewGDBDebugStubConfiguration creates a new GDB debug confiuration.
//
// This API is not officially published and is subject to change without notice.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewGDBDebugStubConfiguration(port uint32) (*GDBDebugStubConfiguration, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	config := &GDBDebugStubConfiguration{
		pointer: pointer{
			ptr: C.newVZGDBDebugStubConfiguration(C.uint32_t(port)),
		},
	}
	runtime.SetFinalizer(config, func(self *GDBDebugStubConfiguration) {
		self.Release()
	})
	return config, nil
}

// SetDebugStubVirtualMachineConfiguration sets debug stub configuration. Empty by default.
//
// This API is not officially published and is subject to change without notice.
func (v *VirtualMachineConfiguration) SetDebugStubVirtualMachineConfiguration(dc DebugStubConfiguration) {
	C.setDebugStubVZVirtualMachineConfiguration(v.Ptr(), dc.Ptr())
}