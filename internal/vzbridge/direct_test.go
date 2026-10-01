//go:build darwin

package vzbridge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

func directTestString(t *testing.T, object objc.NSObject) string {
	t.Helper()
	data := NSString_DataUsingEncoding(object, NSUTF8StringEncoding)
	if data == nil {
		t.Fatal("string conversion returned nil")
	}
	defer objc.Release(data)
	value := string(unsafe.Slice((*byte)(NSData_Bytes(data)), int(NSData_Length(data))))
	runtime.KeepAlive(data)
	return value
}

func TestDirectBorrowedResultSurvivesOwnerRelease(t *testing.T) {
	array := NSMutableArray_New()
	mac := VZMACAddress_InitWithString(NSString_StringWithUTF8String("02:00:00:00:00:17"))
	if mac == nil {
		t.Fatal("valid MAC address was rejected")
	}
	NSMutableArray_AddObject(array, mac)
	got := NSArray_ObjectAtIndex(array, uint64(0))
	objc.Release(mac)
	objc.Release(array)
	Drain()
	runtime.GC()
	text := VZMACAddress_String(got)
	if value := directTestString(t, text); value != "02:00:00:00:00:17" {
		t.Fatalf("MAC address = %q", value)
	}
	objc.Release(text)
	objc.Release(got)
	Drain()
}

func TestDirectNSErrorSurvivesAutoreleasePool(t *testing.T) {
	var nativeError unsafe.Pointer
	value := VZDiskImageStorageDeviceAttachment_InitWithURL_ReadOnly_Error(NSURL_FileURLWithPath(NSString_StringWithUTF8String("/nonexistent/vz-direct-binding/disk.img")), bool(true), &nativeError)
	if value != nil {
		objc.Release(value)
		t.Fatal("missing disk was accepted")
	}
	if nativeError == nil {
		t.Fatal("missing disk returned no NSError")
	}
	failure := objc.NewManagedPointer(nativeError, ReleaseObject)
	description := NSError_LocalizedDescription(failure)
	if got := directTestString(t, description); got == "" {
		t.Fatal("NSError lost its description")
	}
	objc.Release(description)
	objc.Release(failure)
	Drain()
}

func TestDirectValidationRejectsInvalidInput(t *testing.T) {
	var nativeError unsafe.Pointer
	config := NewVZVirtioFileSystemDeviceConfiguration(strings.Repeat("x", 256), &nativeError)
	if config != nil {
		objc.Release(config)
		t.Fatal("oversized filesystem tag was accepted")
	}
	if nativeError == nil {
		t.Fatal("invalid tag returned no NSError")
	}
	ReleaseObject(nativeError)
	nativeError = nil
	config = NewVZDiskImageStorageDeviceAttachmentWithCacheAndSyncMode("/unused", false, -1, 1, &nativeError)
	if config != nil || nativeError == nil {
		t.Fatal("invalid caching mode was accepted")
	}
	failure := objc.NewManagedPointer(nativeError, ReleaseObject)
	if got := NSError_Code(failure); got != 22 {
		t.Fatalf("invalid mode error code = %d, want EINVAL", got)
	}
	objc.Release(failure)
	Drain()
}

func TestDirectPropertiesUseObjectiveCSelectors(t *testing.T) {
	attachment := VZNATNetworkDeviceAttachment_Init()
	config := VZVirtioNetworkDeviceConfiguration_Init()
	VZNetworkDeviceConfiguration_SetAttachment(config, attachment)
	mac := VZMACAddress_InitWithString(NSString_StringWithUTF8String("02:00:00:00:00:29"))
	VZNetworkDeviceConfiguration_SetMACAddress(config, mac)
	got := VZNetworkDeviceConfiguration_MACAddress(config)
	text := VZMACAddress_String(got)
	if value := directTestString(t, text); value != "02:00:00:00:00:29" {
		t.Fatalf("MAC property = %q", value)
	}
	for _, object := range []*objc.Pointer{text, got, mac, config, attachment} {
		objc.Release(object)
	}
	Drain()
}

func TestDirectPrivateInitializerRejectsWrongReceiver(t *testing.T) {
	object := NSMutableArray_New()
	invoked := true
	result := UnsafePrivate__VZGDBDebugStubConfiguration_Instance_initWithPort__be82df9e(objc.Ptr(object), 1234, false, unsafe.Pointer(&invoked))
	if invoked || result != nil {
		t.Fatal("private initializer accepted NSArray receiver")
	}
	if raw := UnsafePrivateAllocate("VZMissingClassForBindingTest"); raw != nil {
		ReleaseObject(raw)
		t.Fatal("missing class allocated an object")
	}
	objc.Release(object)
	Drain()
}

func TestDiskModesFollowSDKValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.img")
	if err := os.WriteFile(path, make([]byte, 512), 0600); err != nil {
		t.Fatal(err)
	}
	for _, caching := range []int32{VZDiskImageCachingModeAutomatic, VZDiskImageCachingModeUncached, VZDiskImageCachingModeCached} {
		for _, synchronization := range []int32{VZDiskImageSynchronizationModeFull, VZDiskImageSynchronizationModeFsync, VZDiskImageSynchronizationModeNone} {
			var nativeError unsafe.Pointer
			attachment := NewVZDiskImageStorageDeviceAttachmentWithCacheAndSyncMode(path, false, caching, synchronization, &nativeError)
			if nativeError != nil {
				ReleaseObject(nativeError)
			}
			if attachment == nil || nativeError != nil {
				t.Fatalf("SDK caching %d / synchronization %d rejected", caching, synchronization)
			}
			objc.Release(attachment)
		}
	}
	Drain()
}
