//go:build darwin && debug
// +build darwin,debug

package vz

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// DebugStubConfiguration is an interface to debug configuration.
type DebugStubConfiguration interface {
	objc.NSObject

	debugStubConfiguration()
}

type baseDebugStubConfiguration struct{}

func (*baseDebugStubConfiguration) debugStubConfiguration() {}

// GDBDebugStubConfiguration is a configuration for gdb debugging.
type GDBDebugStubConfiguration struct {
	*pointer

	*baseDebugStubConfiguration
}

var _ DebugStubConfiguration = (*GDBDebugStubConfiguration)(nil)

// NewGDBDebugStubConfiguration creates a new GDB debug confiuration.
//
// This API is not officially published and is subject to change without notice.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewGDBDebugStubConfiguration(port uint32) (*GDBDebugStubConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	allocation := vzbridge.UnsafePrivateAllocate("_VZGDBDebugStubConfiguration")
	if allocation == nil {
		return nil, fmt.Errorf("private GDB debug configuration is unavailable")
	}
	var invoked bool
	raw := vzbridge.UnsafePrivate__VZGDBDebugStubConfiguration_Instance_initWithPort__be82df9e(allocation, uint16(port), false, unsafe.Pointer(&invoked))
	// An initializer consumes its receiver even when it returns nil.
	if !invoked {
		vzbridge.ReleaseObject(allocation)
		return nil, fmt.Errorf("private GDB debug initializer is unavailable")
	}
	object := objc.NewManagedPointer(raw, vzbridge.ReleaseObject)
	if object == nil {
		return nil, fmt.Errorf("private GDB debug configuration is unavailable")
	}
	return &GDBDebugStubConfiguration{pointer: object}, nil
}

// SetDebugStubVirtualMachineConfiguration sets debug stub configuration. Empty by default.
//
// This API is not officially published and is subject to change without notice.
func (v *VirtualMachineConfiguration) SetDebugStubVirtualMachineConfiguration(dc DebugStubConfiguration) {
	vzbridge.UnsafePrivate_VZVirtualMachineConfiguration_Instance__setDebugStub__cc718b50(objc.Ptr(v), objc.Ptr(dc))
	runtime.KeepAlive(v)
	runtime.KeepAlive(dc)
}
