//go:build darwin

package vzbridge

import (
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	runtimeobjc "github.com/ebitengine/purego/objc"
)

func TestPrivateRuntimeGuardChecksEncoding(t *testing.T) {
	object := NSMutableArray_New()
	defer objc.Release(object)
	selector := runtimeobjc.RegisterName("count")
	if !privateAvailable(objc.Ptr(object), "NSArray", selector, false, "Q", []string{"@", ":"}) {
		t.Fatal("NSArray count rejected its actual ABI")
	}
	if privateAvailable(objc.Ptr(object), "NSArray", selector, false, "I", []string{"@", ":"}) {
		t.Fatal("NSArray count accepted a narrowed return ABI")
	}
	if privateAvailable(objc.Ptr(object), "NSArray", selector, false, "Q", []string{"@", ":", "@"}) {
		t.Fatal("NSArray count accepted an extra argument")
	}
	if privateAvailable(objc.Ptr(object), "NSArray", selector, true, "Q", []string{"@", ":"}) {
		t.Fatal("instance receiver accepted as a class receiver")
	}
	if privateAvailable(objc.Ptr(object), "NSArray", runtimeobjc.RegisterName("vz_missingPrivateMethod"), false, "v", []string{"@", ":"}) {
		t.Fatal("missing selector accepted")
	}
}

func TestPrivateGeneratedGDBConfiguration(t *testing.T) {
	allocation := UnsafePrivateAllocate("_VZGDBDebugStubConfiguration")
	if allocation == nil {
		t.Fatal("GDB configuration class is unavailable")
	}
	invoked := false
	object := UnsafePrivate__VZGDBDebugStubConfiguration_Instance_initWithPort__be82df9e(allocation, 1234, false, unsafe.Pointer(&invoked))
	if !invoked {
		ReleaseObject(allocation)
		t.Fatal("GDB initializer was rejected")
	}
	if object == nil {
		t.Fatal("GDB initializer returned nil")
	}
	config := VZVirtualMachineConfiguration_New()
	UnsafePrivate_VZVirtualMachineConfiguration_Instance__setDebugStub__cc718b50(objc.Ptr(config), object)
	if got := UnsafePrivate__VZGDBDebugStubConfiguration_Instance_port_21ffbdf5(object); got != 1234 {
		t.Fatalf("GDB port = %d, want 1234", got)
	}
	retained := UnsafePrivate_VZVirtualMachineConfiguration_Instance__debugStub_1d6e8719(objc.Ptr(config), true, unsafe.Pointer(&invoked))
	if !invoked || retained != object {
		t.Fatal("generated GDB setter did not store the configuration")
	}
	ReleaseObject(object)
	objc.Release(config)
	Drain()
	defer ReleaseObject(retained)
	if got := UnsafePrivate__VZGDBDebugStubConfiguration_Instance_port_21ffbdf5(retained); got != 1234 {
		t.Fatalf("retained GDB port = %d, want 1234", got)
	}
}

func TestPrivateGeneratedClassMethod(t *testing.T) {
	if got := UnsafePrivate_VZVirtualMachineConfiguration_Class_minimumAllowedCPUCount_80c4b8e4(); got != 1 {
		t.Fatalf("minimum CPU count = %d, want 1", got)
	}
}
