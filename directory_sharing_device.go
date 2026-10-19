package vz

import (
	"fmt"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// DirectorySharingDevice is a directory sharing device attached to a virtual machine.
type DirectorySharingDevice interface {
	objc.NSObject

	directorySharingDevice()
}

// VirtioFileSystemDevice exposes host directories to a guest through Virtio FS.
// Obtain it from VirtualMachine.DirectorySharingDevices.
type VirtioFileSystemDevice struct {
	*pointer
	vm *VirtualMachine
}

func (*VirtioFileSystemDevice) directorySharingDevice() {}

// DirectorySharingDevices returns the directory sharing devices configured on the VM.
// It returns nil on macOS versions older than 12.
func (v *VirtualMachine) DirectorySharingDevices() []DirectorySharingDevice {
	if macOSAvailable(12) != nil {
		return nil
	}
	pointers := nativeArray(vzbridge.VZVirtualMachine_DirectorySharingDevices(v))
	devices := make([]DirectorySharingDevice, len(pointers))
	for i, ptr := range pointers {
		devices[i] = &VirtioFileSystemDevice{
			pointer: ptr,
			vm:      v,
		}
	}
	return devices
}

// Tag returns the label used by the guest to mount this device.
func (d *VirtioFileSystemDevice) Tag() string {
	return objcutil.String(vzbridge.VZVirtioFileSystemDevice_Tag(d))
}

// DirectoryShare returns the directories currently exposed to the guest.
// It returns nil if this device has no directory share.
func (d *VirtioFileSystemDevice) DirectoryShare() DirectoryShare {
	ptr := vzbridge.VZVirtioFileSystemDevice_Share(d)
	if ptr == nil {
		return nil
	}
	switch objcutil.String(vzbridge.NSObject_ClassName(ptr)) {
	case "VZSingleDirectoryShare":
		return &SingleDirectoryShare{pointer: ptr}
	case "VZMultipleDirectoryShare":
		return &MultipleDirectoryShare{pointer: ptr}
	case "VZLinuxRosettaDirectoryShare":
		return wrapLinuxRosettaDirectoryShare(ptr)
	default:
		return nil
	}
}

// SetDirectoryShare changes the directories exposed to the guest, including while
// the VM is running. A nil share removes access to the shared directories.
// Only SingleDirectoryShare and MultipleDirectoryShare are supported. Rosetta
// shares must be assigned to the device configuration before creating the VM.
// This requires macOS 12 or newer.
func (d *VirtioFileSystemDevice) SetDirectoryShare(share DirectoryShare) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	switch share.(type) {
	case nil, *SingleDirectoryShare, *MultipleDirectoryShare:
	default:
		return fmt.Errorf("unsupported runtime directory share %T", share)
	}
	vzbridge.VZVirtioFileSystemDevice_SetShare(d, share)
	return nil
}
