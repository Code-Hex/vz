package vz

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization -framework Cocoa
# include "virtualization_11.h"
# include "virtualization_12.h"
# include "virtualization_13.h"
*/
import "C"
import (
	"fmt"
	"runtime/cgo"
	"sync"
	"unsafe"

	infinity "github.com/Code-Hex/go-infinity-channel"
	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/sliceutil"
)

// VirtualMachineState represents execution state of the virtual machine.
//
//go:generate stringer -type=VirtualMachineState
type VirtualMachineState int

const (
	// VirtualMachineStateStopped Initial state before the virtual machine is started.
	VirtualMachineStateStopped VirtualMachineState = iota

	// VirtualMachineStateRunning Running virtual machine.
	VirtualMachineStateRunning

	// VirtualMachineStatePaused A started virtual machine is paused.
	// This state can only be transitioned from VirtualMachineStatePausing.
	VirtualMachineStatePaused

	// VirtualMachineStateError The virtual machine has encountered an internal error.
	VirtualMachineStateError

	// VirtualMachineStateStarting The virtual machine is configuring the hardware and starting.
	VirtualMachineStateStarting

	// VirtualMachineStatePausing The virtual machine is being paused.
	// This is the intermediate state between VirtualMachineStateRunning and VirtualMachineStatePaused.
	VirtualMachineStatePausing

	// VirtualMachineStateResuming The virtual machine is being resumed.
	// This is the intermediate state between VirtualMachineStatePaused and VirtualMachineStateRunning.
	VirtualMachineStateResuming

	// VirtualMachineStateStopping The virtual machine is being stopped.
	// This is the intermediate state between VirtualMachineStateRunning and VirtualMachineStateStop.
	//
	// Available on macOS 12.0 and above.
	VirtualMachineStateStopping

	// VirtualMachineStateSaving The virtual machine is being saved.
	// This is the intermediate state between VirtualMachineStatePaused and VirtualMachineStatePaused
	//
	// Available on macOS 14.0 and above.
	VirtualMachineStateSaving

	// VirtualMachineStateRestoring	The virtual machine is being restored.
	// This is the intermediate state between VirtualMachineStateStopped and either VirtualMachineStatePaused on success or VirtualMachineStateStopped on failure.
	//
	// Available on macOS 14.0 and above.
	VirtualMachineStateRestoring
)

// VirtualMachine represents the entire state of a single virtual machine.
//
// A Virtual Machine is the emulation of a complete hardware machine of the same architecture as the real hardware machine.
// When executing the Virtual Machine, the Virtualization framework uses certain hardware resources and emulates others to provide isolation
// and great performance.
//
// The definition of a virtual machine starts with its configuration. This is done by setting up a VirtualMachineConfiguration struct.
// Once configured, the virtual machine can be started with (*VirtualMachine).Start() method.
//
// Creating a virtual machine using the Virtualization framework requires the app to have the "com.apple.security.virtualization" entitlement.
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine?language=objc
type VirtualMachine struct {
	// id for this struct.
	id string

	// Indicate whether or not virtualization is available.
	//
	// If virtualization is unavailable, no VirtualMachineConfiguration will validate.
	// The validation error of the VirtualMachineConfiguration provides more information about why virtualization is unavailable.
	supported bool

	*pointer
	dispatchQueue unsafe.Pointer
	machineState  *machineState

	disconnectedIn        *infinity.Channel[*disconnected]
	disconnectedOut       *infinity.Channel[*DisconnectedError]
	watchDisconnectedOnce sync.Once

	finalizeOnce sync.Once

	config *VirtualMachineConfiguration

	mu sync.RWMutex
}

type machineState struct {
	state       VirtualMachineState
	stateNotify *infinity.Channel[VirtualMachineState]

	mu sync.RWMutex
}

// NewVirtualMachine creates a new VirtualMachine with VirtualMachineConfiguration.
//
// The configuration must be valid. Validation can be performed at runtime with (*VirtualMachineConfiguration).Validate() method.
// The configuration is copied by the initializer.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtualMachine(config *VirtualMachineConfiguration) (*VirtualMachine, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	// should not call Free function for this string.
	cs := (*char)(objc.GetUUID())
	dispatchQueue := C.makeDispatchQueue(cs.CString())

	machineState := &machineState{
		state:       VirtualMachineState(0),
		stateNotify: infinity.NewChannel[VirtualMachineState](),
	}
	stateHandle := cgo.NewHandle(machineState)

	disconnectedIn := infinity.NewChannel[*disconnected]()
	disconnectedOut := infinity.NewChannel[*DisconnectedError]()
	disconnectedHandle := cgo.NewHandle(disconnectedIn)

	v := &VirtualMachine{
		id: cs.String(),
		pointer: objc.NewPointer(
			C.newVZVirtualMachineWithDispatchQueue(
				objc.Ptr(config),
				dispatchQueue,
				C.uintptr_t(stateHandle),
				C.uintptr_t(disconnectedHandle),
			),
		),
		dispatchQueue:   dispatchQueue,
		machineState:    machineState,
		disconnectedIn:  disconnectedIn,
		disconnectedOut: disconnectedOut,
		config:          config,
	}

	objc.SetFinalizer(v, func(self *VirtualMachine) {
		self.finalize()
		stateHandle.Delete()
	})
	return v, nil
}

func (v *VirtualMachine) finalize() {
	v.finalizeOnce.Do(func() {
		objc.ReleaseDispatch(v.dispatchQueue)
		objc.Release(v)
	})
}

// SocketDevices return the list of socket devices configured on this virtual machine.
// Return an empty array if no socket device is configured.
//
// Since only NewVirtioSocketDeviceConfiguration is available in vz package,
// it will always return VirtioSocketDevice.
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine/3656702-socketdevices?language=objc
func (v *VirtualMachine) SocketDevices() []*VirtioSocketDevice {
	nsArray := objc.NewNSArray(
		C.VZVirtualMachine_socketDevices(objc.Ptr(v)),
	)
	ptrs := nsArray.ToPointerSlice()
	socketDevices := make([]*VirtioSocketDevice, len(ptrs))
	for i, ptr := range ptrs {
		socketDevices[i] = newVirtioSocketDevice(ptr, v.dispatchQueue)
	}
	return socketDevices
}

//export changeStateOnObserver
func changeStateOnObserver(newStateRaw C.int, cgoHandleUintptr C.uintptr_t) {
	stateHandle := cgo.Handle(cgoHandleUintptr)
	// I expected it will not cause panic.
	// if caused panic, that's unexpected behavior.
	v, _ := stateHandle.Value().(*machineState)
	v.mu.Lock()
	newState := VirtualMachineState(newStateRaw)
	v.state = newState
	v.stateNotify.In() <- newState
	v.mu.Unlock()
}

// State represents execution state of the virtual machine.
func (v *VirtualMachine) State() VirtualMachineState {
	v.machineState.mu.RLock()
	defer v.machineState.mu.RUnlock()
	return v.machineState.state
}

// StateChangedNotify gets notification is changed execution state of the virtual machine.
func (v *VirtualMachine) StateChangedNotify() <-chan VirtualMachineState {
	v.machineState.mu.RLock()
	defer v.machineState.mu.RUnlock()
	return v.machineState.stateNotify.Out()
}

//export emitCanDo
func emitCanDo(can C.bool, cgoHandleUintptr C.uintptr_t) {
	cgoHandle := cgo.Handle(cgoHandleUintptr)
	handler := cgoHandle.Value().(func(bool))
	handler(bool(can))
}

// CanStart returns true if the machine is in a state that can be started.
func (v *VirtualMachine) CanStart() bool {
	h, ch := makeHandler[bool]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	go C.vmCanStart(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-ch
}

// CanPause returns true if the machine is in a state that can be paused.
func (v *VirtualMachine) CanPause() bool {
	h, ch := makeHandler[bool]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	go C.vmCanPause(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-ch
}

// CanResume returns true if the machine is in a state that can be resumed.
func (v *VirtualMachine) CanResume() bool {
	h, ch := makeHandler[bool]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	go C.vmCanResume(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-ch
}

// CanRequestStop returns whether the machine is in a state where the guest can be asked to stop.
func (v *VirtualMachine) CanRequestStop() bool {
	h, ch := makeHandler[bool]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	go C.vmCanRequestStop(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-ch
}

// CanStop returns whether the machine is in a state that can be stopped.
//
// This is only supported on macOS 12 and newer, false will always be returned
// on older versions.
func (v *VirtualMachine) CanStop() bool {
	if err := macOSAvailable(12); err != nil {
		return false
	}
	h, ch := makeHandler[bool]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	go C.vmCanStop(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-ch
}

//export virtualMachineCompletionHandler
func virtualMachineCompletionHandler(cgoHandleUintptr C.uintptr_t, errPtr unsafe.Pointer) {
	cgoHandle := cgo.Handle(cgoHandleUintptr)

	handler := cgoHandle.Value().(func(error))

	if err := newNSError(errPtr); err != nil {
		handler(err)
	} else {
		handler(nil)
	}
}

func makeHandler[T any]() (func(T), chan T) {
	ch := make(chan T, 1)
	return func(v T) {
		ch <- v
		close(ch)
	}, ch
}

type virtualMachineStartOptions struct {
	macOSVirtualMachineStartOptionsPtr unsafe.Pointer
}

// VirtualMachineStartOption is an option for virtual machine start.
type VirtualMachineStartOption func(*virtualMachineStartOptions) error

// Start a virtual machine that is in either Stopped or Error state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
//
// If options are specified, also checks whether these options are
// available in use your macOS version available.
func (v *VirtualMachine) Start(opts ...VirtualMachineStartOption) error {
	o := &virtualMachineStartOptions{}
	for _, optFunc := range opts {
		if err := optFunc(o); err != nil {
			return err
		}
	}

	h, errCh := makeHandler[error]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()

	if o.macOSVirtualMachineStartOptionsPtr != nil {
		C.startWithOptionsCompletionHandler(
			objc.Ptr(v),
			v.dispatchQueue,
			o.macOSVirtualMachineStartOptionsPtr,
			C.uintptr_t(handle),
		)
	} else {
		C.startWithCompletionHandler(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	}
	return <-errCh
}

// Pause a virtual machine that is in Running state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
func (v *VirtualMachine) Pause() error {
	h, errCh := makeHandler[error]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	C.pauseWithCompletionHandler(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-errCh
}

// Resume a virtual machine that is in the Paused state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
func (v *VirtualMachine) Resume() error {
	h, errCh := makeHandler[error]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	C.resumeWithCompletionHandler(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-errCh
}

type requestStopResult struct {
	success bool
	err     *NSError
}

//export emitRequestStop
func emitRequestStop(success C.bool, errPtr unsafe.Pointer, cgoHandleUintptr C.uintptr_t) {
	cgoHandle := cgo.Handle(cgoHandleUintptr)
	handler := cgoHandle.Value().(func(requestStopResult))
	handler(requestStopResult{
		success: bool(success),
		err:     newNSError(errPtr),
	})
}

// RequestStop requests that the guest turns itself off.
//
// If returned error is not nil, assigned with the error if the request failed.
// Returns true if the request was made successfully.
func (v *VirtualMachine) RequestStop() (bool, error) {
	h, ch := makeHandler[requestStopResult]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	go C.requestStopVirtualMachine(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	ret := <-ch
	return ret.success, ret.err
}

// Stop stops a VM that’s in either a running or paused state.
//
// The completion handler returns an error object when the VM fails to stop,
// or nil if the stop was successful.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
//
// Warning: This is a destructive operation. It stops the VM without
// giving the guest a chance to stop cleanly.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) Stop() error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	h, errCh := makeHandler[error]()
	handle := cgo.NewHandle(h)
	defer handle.Delete()
	C.stopWithCompletionHandler(objc.Ptr(v), v.dispatchQueue, C.uintptr_t(handle))
	return <-errCh
}

// StartGraphicApplication starts an application to display graphics of the VM.
//
// You must to call runtime.LockOSThread before calling this method.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) StartGraphicApplication(width, height float64) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	C.startVirtualMachineWindow(objc.Ptr(v), C.double(width), C.double(height))
	return nil
}

// DisconnectedError represents an error that occurs when a VM’s network attachment is disconnected
// due to a network-related issue. This error is triggered by the framework when such a disconnection happens.
type DisconnectedError struct {
	// Err is the underlying error that caused the disconnection, triggered by the framework.
	// This error provides information on why the network attachment was disconnected.
	Err error
	// The network device configuration associated with the disconnection event.
	// This configuration helps identify which network device experienced the disconnection.
	// If Config is nil, the specific configuration details are unavailable.
	Config *VirtioNetworkDeviceConfiguration
}

var _ error = (*DisconnectedError)(nil)

func (e *DisconnectedError) Unwrap() error { return e.Err }
func (e *DisconnectedError) Error() string {
	if e.Config == nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Config.attachment, e.Err)
}

type disconnected struct {
	err   error
	index int
}

// NetworkDeviceAttachmentWasDisconnected returns a receive channel.
// The channel emits an error message each time the network attachment is disconnected,
// typically triggered by events such as failure to start, initial boot, device reset, or reboot.
// As a result, this method may be invoked multiple times throughout the virtual machine's lifecycle.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) NetworkDeviceAttachmentWasDisconnected() (<-chan *DisconnectedError, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	v.watchDisconnectedOnce.Do(func() {
		go v.watchDisconnected()
	})
	return v.disconnectedOut.Out(), nil
}

// TODO(codehex): refactoring to leave using machineState's mutex lock.
func (v *VirtualMachine) watchDisconnected() {
	for disconnected := range v.disconnectedIn.Out() {
		v.mu.RLock()
		config := sliceutil.FindValueByIndex(
			v.config.networkDeviceConfiguration,
			disconnected.index,
		)
		v.mu.RUnlock()
		v.disconnectedOut.In() <- &DisconnectedError{
			Err:    disconnected.err,
			Config: config,
		}
	}
	v.disconnectedOut.Close()
}

//export emitAttachmentWasDisconnected
func emitAttachmentWasDisconnected(index C.int, errPtr unsafe.Pointer, cgoHandleUintptr C.uintptr_t) {
	handler := cgo.Handle(cgoHandleUintptr)
	err := newNSError(errPtr)
	// I expected it will not cause panic.
	// if caused panic, that's unexpected behavior.
	ch, _ := handler.Value().(*infinity.Channel[*disconnected])
	ch.In() <- &disconnected{
		err:   err,
		index: int(index),
	}
}

//export closeAttachmentWasDisconnectedChannel
func closeAttachmentWasDisconnectedChannel(cgoHandleUintptr C.uintptr_t) {
	handler := cgo.Handle(cgoHandleUintptr)
	// I expected it will not cause panic.
	// if caused panic, that's unexpected behavior.
	ch, _ := handler.Value().(*infinity.Channel[*disconnected])
	ch.Close()
}
