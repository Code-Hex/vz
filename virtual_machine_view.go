//go:build darwin

package vz

import (
	"errors"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// VirtualMachineView represents a native virtual machine view embedded in a
// host GUI.
//
// The constructor and all methods must run on the process main thread. Pinning
// a goroutine to another OS thread with runtime.LockOSThread is not enough.
//
// Keep the Go view reachable while it is mounted in the native interface, and
// call [VirtualMachineView.Close] before the host event loop ends. Value copies
// of a view share ownership, so closing any copy closes all of them. Closing a
// view detaches it from the host interface but does not stop the virtual
// machine.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview?language=objc
type VirtualMachineView struct{ owner *virtualMachineViewOwner }
type virtualMachineViewOwner struct {
	native  *objc.Pointer
	machine *VirtualMachine
	borrows int
}

func viewMainThread() error {
	if !vzbridge.ViewIsMainThread() {
		return errors.New("virtual machine view requires the process main thread")
	}
	return nil
}

// NewVirtualMachineView creates a view for machine on macOS 12 or later.
//
// This constructor must run on the process main thread. System keyboard capture
// and automatic display reconfiguration both default to false.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview?language=objc
func NewVirtualMachineView(machine *VirtualMachine) (*VirtualMachineView, error) {
	if machine == nil || machine.pointer == nil || objc.Ptr(machine) == nil {
		return nil, errors.New("virtual machine view requires a valid virtual machine")
	}
	if err := viewMainThread(); err != nil {
		return nil, err
	}
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	native := vzbridge.ViewNew(objc.Ptr(machine))
	runtime.KeepAlive(machine)
	if native == nil {
		return nil, errors.New("failed to create virtual machine view")
	}
	return &VirtualMachineView{
		owner: &virtualMachineViewOwner{
			native:  objc.NewManagedPointer(native, vzbridge.ViewRelease),
			machine: machine,
		},
	}, nil
}

func (v *VirtualMachineView) active() (*virtualMachineViewOwner, error) {
	if err := viewMainThread(); err != nil {
		return nil, err
	}
	if v == nil || v.owner == nil || v.owner.native == nil {
		return nil, errors.New("virtual machine view is closed")
	}
	return v.owner, nil
}

// WithNativeView lends a pointer to the Objective-C VZVirtualMachineView to fn
// for the duration of the call.
//
// The pointer references an Objective-C object rather than Go memory. Callers
// must not save the borrowed pointer, release it, reenter the host event loop,
// or modify the view's native virtualMachine property. Adding the view as a
// subview retains it. Nested calls to WithNativeView are allowed.
//
// An autorelease pool drains before the borrow ends, including when fn panics.
// Autoreleased objects used after fn returns must be retained separately. Any
// error returned by fn or panic raised inside fn propagates unchanged.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview?language=objc
func (v *VirtualMachineView) WithNativeView(fn func(unsafe.Pointer) error) error {
	owner, err := v.active()
	if err != nil {
		return err
	}
	if fn == nil {
		return errors.New("native view callback must not be nil")
	}
	owner.borrows++
	pool := vzbridge.ViewBorrowPoolNew()
	defer func() {
		vzbridge.ViewBorrowPoolDrain(pool)
		owner.borrows--
		runtime.KeepAlive(owner)
	}()
	return fn(objc.Ptr(owner.native))
}

// CapturesSystemKeys reports whether certain system keyboard shortcuts are sent
// to the guest virtual machine instead of the macOS host.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview/capturessystemkeys?language=objc
func (v *VirtualMachineView) CapturesSystemKeys() (bool, error) {
	owner, err := v.active()
	if err != nil {
		return false, err
	}
	value := vzbridge.ViewCapturesKeys(objc.Ptr(owner.native))
	runtime.KeepAlive(owner)
	return value, nil
}

// SetCapturesSystemKeys controls whether certain system keyboard shortcuts are
// sent to the guest virtual machine instead of the macOS host.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview/capturessystemkeys?language=objc
func (v *VirtualMachineView) SetCapturesSystemKeys(value bool) error {
	owner, err := v.active()
	if err != nil {
		return err
	}
	vzbridge.ViewSetCapturesKeys(objc.Ptr(owner.native), value)
	runtime.KeepAlive(owner)
	return nil
}

// AutomaticallyReconfiguresDisplay reports whether the view automatically
// resizes or reconfigures the guest display when the view changes.
//
// This method requires macOS 14 or later. Only one view per display can have
// automatic reconfiguration enabled, so enabling it on another view for the
// same display can disable it on this view.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview/automaticallyreconfiguresdisplay?language=objc
func (v *VirtualMachineView) AutomaticallyReconfiguresDisplay() (bool, error) {
	owner, err := v.active()
	if err != nil {
		return false, err
	}
	if err := macOSAvailable(14); err != nil {
		return false, err
	}
	value := vzbridge.ViewAutomaticallyReconfigures(objc.Ptr(owner.native))
	runtime.KeepAlive(owner)
	return value, nil
}

// SetAutomaticallyReconfiguresDisplay controls whether the view automatically
// resizes or reconfigures the guest display when the view changes.
//
// This method requires macOS 14 or later. Only one view per display can have
// automatic reconfiguration enabled, so enabling it on another view for the
// same display can disable it on this view.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview/automaticallyreconfiguresdisplay?language=objc
func (v *VirtualMachineView) SetAutomaticallyReconfiguresDisplay(value bool) error {
	owner, err := v.active()
	if err != nil {
		return err
	}
	if err := macOSAvailable(14); err != nil {
		return err
	}
	vzbridge.ViewSetAutomaticallyReconfigures(objc.Ptr(owner.native), value)
	runtime.KeepAlive(owner)
	return nil
}

// Close detaches the view from its superview and releases the reference owned
// by vz.
//
// Calling Close on the process main thread succeeds repeatedly and is a no-op
// on a nil receiver or zero-value [VirtualMachineView]. If a native pointer is
// currently borrowed through [VirtualMachineView.WithNativeView], Close returns
// an error and leaves the view open.
//
// Close does not stop the virtual machine. Other native owners and pending
// framework operations may keep the native view and virtual machine alive, so
// Close does not guarantee final destruction.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineview?language=objc
func (v *VirtualMachineView) Close() error {
	if err := viewMainThread(); err != nil {
		return err
	}
	if v == nil || v.owner == nil || v.owner.native == nil {
		return nil
	}
	owner := v.owner
	if owner.borrows != 0 {
		return errors.New("cannot close virtual machine view during a native borrow")
	}
	native := owner.native
	owner.native = nil
	objc.Release(native)
	owner.machine = nil
	runtime.KeepAlive(owner)
	return nil
}
