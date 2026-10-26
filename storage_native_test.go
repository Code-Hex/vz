package vz_test

import (
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestDiskImageStorageDeviceAttachmentFromNative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := vz.CreateDiskImage(path, 512); err != nil {
		t.Fatal(err)
	}
	original, err := vz.NewDiskImageStorageDeviceAttachment(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(original)

	attachment, err := vz.NewDiskImageStorageDeviceAttachmentFromNative(objc.Ptr(original))
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(attachment)
	objc.Release(original)
	vzbridge.Drain()

	if !vzbridge.VZDiskImageStorageDeviceAttachment_IsReadOnly(attachment) {
		t.Fatal("attachment lost its read-only setting")
	}
	url := vzbridge.VZDiskImageStorageDeviceAttachment_URL(attachment)
	defer objc.Release(url)
	nativePath := vzbridge.NSURL_Path(url)
	defer objc.Release(nativePath)
	if got := vzbridge.NSString_UTF8String(nativePath); got != path {
		t.Fatalf("disk path = %q, want %q", got, path)
	}

	config, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(config)
	configuredAttachment := vzbridge.VZStorageDeviceConfiguration_Attachment(config)
	defer objc.Release(configuredAttachment)
	if objc.Ptr(configuredAttachment) != objc.Ptr(attachment) {
		t.Fatal("native storage configuration lost its attachment")
	}
}

func TestDiskImageStorageDeviceAttachmentFromNativeRejectsInvalidObject(t *testing.T) {
	wrongClass := vzbridge.NSMutableArray_New()
	defer objc.Release(wrongClass)
	for _, pointer := range []unsafe.Pointer{nil, objc.Ptr(wrongClass)} {
		attachment, err := vz.NewDiskImageStorageDeviceAttachmentFromNative(pointer)
		if attachment != nil || err == nil {
			t.Fatalf("invalid object returned attachment %v and error %v", attachment, err)
		}
	}
	vzbridge.Drain()
	if count := vzbridge.NSArray_Count(wrongClass); count != 0 {
		t.Fatalf("rejected object count = %d, want 0", count)
	}
}
