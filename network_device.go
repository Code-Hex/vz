package vz

import (
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// NetworkDevice is a network device attached to a virtual machine.
// Obtain it from VirtualMachine.NetworkDevices.
type NetworkDevice struct {
	*pointer
	vm *VirtualMachine
}

// NetworkDevices returns the network devices configured on the VM.
// It returns nil on macOS versions older than 12.
func (v *VirtualMachine) NetworkDevices() []*NetworkDevice {
	if macOSAvailable(12) != nil {
		return nil
	}
	pointers := nativeArray(vzbridge.VZVirtualMachine_NetworkDevices(v))
	devices := make([]*NetworkDevice, len(pointers))
	for i, ptr := range pointers {
		devices[i] = &NetworkDevice{
			pointer: ptr,
			vm:      v,
		}
	}
	return devices
}

// Attachment returns the network attachment currently connected to the device.
// It returns nil when disconnected. The attachment can change while the VM is
// running, including after a connection failure.
func (d *NetworkDevice) Attachment() NetworkDeviceAttachment {
	ptr := vzbridge.VZNetworkDevice_Attachment(d)
	if ptr == nil {
		return nil
	}
	switch objcutil.String(vzbridge.NSObject_ClassName(ptr)) {
	case "VZNATNetworkDeviceAttachment":
		return &NATNetworkDeviceAttachment{pointer: ptr}
	case "VZBridgedNetworkDeviceAttachment":
		return &BridgedNetworkDeviceAttachment{pointer: ptr}
	case "VZFileHandleNetworkDeviceAttachment":
		return &FileHandleNetworkDeviceAttachment{pointer: ptr}
	case "VZVmnetNetworkDeviceAttachment":
		return &VMNetNetworkDeviceAttachment{pointer: ptr}
	default:
		return nil
	}
}

// SetAttachment changes the network attachment, including while the VM is
// running. A nil attachment disconnects the device.
//
// A nil error does not guarantee that the attachment connected successfully.
// Connection failures are reported through
// VirtualMachine.NetworkDeviceAttachmentWasDisconnectedNotify and clear the attachment.
// DisconnectedError.Config identifies the original device configuration; it does
// not track attachment changes made through this method.
// This requires macOS 12 or newer.
func (d *NetworkDevice) SetAttachment(attachment NetworkDeviceAttachment) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	vzbridge.VZNetworkDevice_SetAttachment(d, attachment)
	return nil
}
