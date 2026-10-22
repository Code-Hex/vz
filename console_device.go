package vz

import (
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// ConsolePortEvent reports a guest opening or closing a console port.
type ConsolePortEvent struct {
	// DeviceIndex identifies the console device in VirtualMachine.ConsoleDevices.
	DeviceIndex int
	// PortIndex is the key in VirtioConsoleDevice.Ports.
	PortIndex uint32
	// Open reports whether the guest opened or closed the port.
	Open bool
}

// VirtioConsoleDevice is a console device attached to a virtual machine.
// Obtain it from VirtualMachine.ConsoleDevices.
type VirtioConsoleDevice struct {
	*pointer
	vm *VirtualMachine
}

// ConsolePortChangedNotify reports guest console port opens and closes in order.
// Notifications are queued from VM creation, including before this method is called.
// Process or flush pending attachment data before communicating with a new guest
// process. Closing a port does not guarantee that its output has been consumed.
// Releasing the VM closes the channel and discards unread notifications. Keep the
// VM reachable until you finish receiving, for example with runtime.KeepAlive.
// It returns nil on macOS versions older than 13.
func (v *VirtualMachine) ConsolePortChangedNotify() <-chan ConsolePortEvent {
	if macOSAvailable(13) != nil {
		return nil
	}
	return v.machineState.consolePorts.output
}

// ConsoleDevices returns the console devices configured on the VM.
// It returns nil on macOS versions older than 13.
func (v *VirtualMachine) ConsoleDevices() []*VirtioConsoleDevice {
	if macOSAvailable(13) != nil {
		return nil
	}
	pointers := nativeArray(vzbridge.VZVirtualMachine_ConsoleDevices(v))
	devices := make([]*VirtioConsoleDevice, len(pointers))
	for i, ptr := range pointers {
		devices[i] = &VirtioConsoleDevice{
			pointer: ptr,
			vm:      v,
		}
	}
	return devices
}

// Ports returns the configured ports keyed by their configuration indexes.
// Unconfigured indexes are absent. Changing the map does not change the device.
func (d *VirtioConsoleDevice) Ports() map[uint32]*VirtioConsolePort {
	array := vzbridge.VZVirtioConsoleDevice_Ports(d)
	defer objc.Release(array)
	ports := make(map[uint32]*VirtioConsolePort)
	count := vzbridge.VZVirtioConsolePortArray_MaximumPortCount(array)
	for index := uint32(0); index < count; index++ {
		ptr := vzbridge.VZVirtioConsolePortArray_ObjectAtIndexedSubscript(array, uint64(index))
		if ptr != nil {
			ports[index] = &VirtioConsolePort{
				pointer: ptr,
				vm:      d.vm,
			}
		}
	}
	return ports
}

// VirtioConsolePort is a port on a running or stopped virtual machine.
// Obtain it from VirtioConsoleDevice.Ports.
type VirtioConsolePort struct {
	*pointer
	vm *VirtualMachine
}

// Name returns the configured port name, or an empty string for an unnamed port.
func (p *VirtioConsolePort) Name() string {
	return objcutil.String(vzbridge.VZVirtioConsolePort_Name(p))
}

// Attachment returns the serial attachment currently connected to the port.
// It returns nil when disconnected.
func (p *VirtioConsolePort) Attachment() SerialPortAttachment {
	ptr := vzbridge.VZVirtioConsolePort_Attachment(p)
	if ptr == nil {
		return nil
	}
	switch objcutil.String(vzbridge.NSObject_ClassName(ptr)) {
	case "VZFileHandleSerialPortAttachment":
		return &FileHandleSerialPortAttachment{pointer: ptr}
	case "VZFileSerialPortAttachment":
		return &FileSerialPortAttachment{pointer: ptr}
	default:
		return nil
	}
}

// SetAttachment changes the port attachment, including while the VM is running.
// A nil attachment disconnects the port. This requires macOS 13 or newer.
func (p *VirtioConsolePort) SetAttachment(attachment SerialPortAttachment) error {
	if err := macOSAvailable(13); err != nil {
		return err
	}
	vzbridge.VZVirtioConsolePort_SetAttachment(p, attachment)
	return nil
}
