package vz

import (
	"reflect"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
)

func TestRuntimeDirectorySharingDevice(t *testing.T) {
	if macOSAvailable(12) != nil {
		t.Skip("requires macOS 12")
	}
	shared, err := NewSharedDirectory(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	single, err := NewSingleDirectoryShare(shared)
	if err != nil {
		t.Fatal(err)
	}
	multiple, err := NewMultipleDirectoryShare(map[string]*SharedDirectory{"shared": shared})
	if err != nil {
		t.Fatal(err)
	}
	fileSystem, err := NewVirtioFileSystemDeviceConfiguration("runtime-share")
	if err != nil {
		t.Fatal(err)
	}
	fileSystem.SetDirectoryShare(single)
	config := newTestConfig(t)
	config.SetDirectorySharingDevicesVirtualMachineConfiguration([]DirectorySharingDeviceConfiguration{fileSystem})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	devices := vm.DirectorySharingDevices()
	if len(devices) != 1 {
		t.Fatalf("got %d directory sharing devices, want 1", len(devices))
	}
	device := devices[0].(*VirtioFileSystemDevice)
	if got := device.Tag(); got != "runtime-share" {
		t.Fatalf("tag = %q, want runtime-share", got)
	}
	for _, share := range []DirectoryShare{multiple, single, nil} {
		if err := device.SetDirectoryShare(share); err != nil {
			t.Fatal(err)
		}
		got := device.DirectoryShare()
		if reflect.TypeOf(got) != reflect.TypeOf(share) {
			t.Errorf("directory share type = %T, want %T", got, share)
		}
		if objc.Ptr(got) != objc.Ptr(share) {
			t.Errorf("directory share did not change to %T", share)
		}
		if got != nil {
			objc.Release(got)
		}
	}
}
