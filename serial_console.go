package vz

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// SerialPortAttachment interface for a serial port attachment.
//
// A serial port attachment defines how the virtual machine's serial port interfaces with the host system.
type SerialPortAttachment interface {
	objc.NSObject

	serialPortAttachment()
}

type baseSerialPortAttachment struct{}

func (*baseSerialPortAttachment) serialPortAttachment() {}

var _ SerialPortAttachment = (*FileHandleSerialPortAttachment)(nil)

// ErrSerialPortAttachmentClosed reports that a serial attachment has begun closing
// or has already been closed.
// It wraps [os.ErrClosed].
var ErrSerialPortAttachmentClosed = fmt.Errorf("serial port attachment: %w", os.ErrClosed)

// ErrSerialPortAttachmentInUse is returned when a virtual machine is using the attachment.
var ErrSerialPortAttachmentInUse = errors.New("serial port attachment is in use by a virtual machine")

// FileHandleSerialPortAttachment defines a serial port attachment from a file handle.
//
// Data written to fileHandleForReading goes to the guest. Data sent from the guest appears on fileHandleForWriting.
// see: https://developer.apple.com/documentation/virtualization/vzfilehandleserialportattachment?language=objc
type FileHandleSerialPortAttachment struct {
	*pointer

	*baseSerialPortAttachment
}

// NewFileHandleSerialPortAttachment creates a serial attachment from file handles.
// Data read from read is sent to the guest. Data from the guest is written to write.
// Either file may be nil to disable that direction, and both may be nil.
//
// The attachment duplicates each non-nil file descriptor. The caller retains
// ownership of the original files and may close them after this call returns.
// A closed file returns an error.
//
// This is only supported on macOS 11 and newer. An error is returned on older versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzfilehandleserialportattachment/init(filehandleforreading:filehandleforwriting:)?language=objc
func NewFileHandleSerialPortAttachment(read, write *os.File) (*FileHandleSerialPortAttachment, error) {
	defer runtime.KeepAlive(read)
	defer runtime.KeepAlive(write)
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	var nserrPtr unsafe.Pointer
	attachment := &FileHandleSerialPortAttachment{
		pointer: vzbridge.NewVZFileHandleSerialPortAttachment(
			int32(read.Fd()),
			read != nil,
			int32(write.Fd()),
			write != nil,
			&nserrPtr,
		),
	}
	if err := objcutil.NewNSError(nserrPtr); err != nil {
		return nil, err
	}
	return attachment, nil
}

// Close closes the file descriptors owned by the attachment. The caller retains
// ownership of the original files passed to NewFileHandleSerialPortAttachment.
//
// Close returns [ErrSerialPortAttachmentInUse] while any connected VM is
// running, paused, or transitioning between states. Wait for shutdown or
// disconnect the attachment before closing it. A VM has shut down once
// [VirtualMachine.State] reports [VirtualMachineStateStopped] or
// [VirtualMachineStateError] and no start, restore, or macOS install call on
// it is still in progress, whether the host or the guest stopped it.
// To restart a VM configured with a legacy serial port after Close, create a
// VM with a fresh attachment. A runtime console port can be given a fresh
// attachment with [VirtioConsolePort.SetAttachment].
//
// For pipe output, close the original writing end after creating the attachment
// and read the output concurrently while the guest runs. After the VM stops,
// call Close before waiting for the reader to reach EOF.
//
// Close attempts to close both directions and returns the first error, if any.
// If a file handle fails to close, another call retries the remaining handles.
// After a successful close, subsequent calls return [ErrSerialPortAttachmentClosed].
func (a *FileHandleSerialPortAttachment) Close() error {
	if a == nil || a.pointer == nil {
		return nil
	}
	var errorPointer unsafe.Pointer
	if alreadyClosed := vzbridge.CloseVZFileHandleSerialPortAttachment(a, &errorPointer); alreadyClosed {
		return ErrSerialPortAttachmentClosed
	}
	return serialAttachmentError(errorPointer)
}

func serialAttachmentError(errorPointer unsafe.Pointer) error {
	err := objcutil.NewNSError(errorPointer)
	if err == nil {
		return nil
	}
	if err.Domain == vzbridge.SerialAttachmentErrorDomain {
		switch err.Code {
		case vzbridge.ErrorSerialAttachmentClosed:
			return ErrSerialPortAttachmentClosed
		case vzbridge.ErrorSerialAttachmentInUse:
			return ErrSerialPortAttachmentInUse
		}
	}
	return err
}

var _ SerialPortAttachment = (*FileSerialPortAttachment)(nil)

// FileSerialPortAttachment defines a serial port attachment from a file.
//
// Any data sent by the guest on the serial interface is written to the file.
// No data is sent to the guest over serial with this attachment.
// see: https://developer.apple.com/documentation/virtualization/vzfileserialportattachment?language=objc
type FileSerialPortAttachment struct {
	*pointer

	*baseSerialPortAttachment
}

// NewFileSerialPortAttachment initialize the FileSerialPortAttachment from a path of a file.
// If error is not nil, used to report errors if intialization fails.
//
//   - path of the file for the attachment on the local file system.
//   - shouldAppend True if the file should be opened in append mode, false otherwise.
//     When a file is opened in append mode, writing to that file will append to the end of it.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewFileSerialPortAttachment(path string, shouldAppend bool) (*FileSerialPortAttachment, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	var nserrPtr unsafe.Pointer
	attachment := &FileSerialPortAttachment{
		pointer: vzbridge.VZFileSerialPortAttachment_InitWithURL_Append_Error(vzbridge.NSURL_FileURLWithPath(vzbridge.NSString_StringWithUTF8String(path)), bool(shouldAppend), &nserrPtr),
	}
	if err := objcutil.NewNSError(nserrPtr); err != nil {
		return nil, err
	}
	return attachment, nil
}

// VirtioConsoleDeviceSerialPortConfiguration represents Virtio Console Serial Port Device.
//
// The device creates a console which enables communication between the host and the guest through the Virtio interface.
// The device sets up a single port on the Virtio console device.
// see: https://developer.apple.com/documentation/virtualization/vzvirtioconsoledeviceserialportconfiguration?language=objc
type VirtioConsoleDeviceSerialPortConfiguration struct {
	*pointer
}

// NewVirtioConsoleDeviceSerialPortConfiguration creates a new NewVirtioConsoleDeviceSerialPortConfiguration.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtioConsoleDeviceSerialPortConfiguration(attachment SerialPortAttachment) (*VirtioConsoleDeviceSerialPortConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	config := &VirtioConsoleDeviceSerialPortConfiguration{
		pointer: vzbridge.VZVirtioConsoleDeviceSerialPortConfiguration_Init(),
	}
	vzbridge.VZSerialPortConfiguration_SetAttachment(config, attachment)
	return config, nil
}
