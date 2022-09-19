package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization -framework Cocoa
# include "virtualization.h"
*/
import "C"
import (
	"runtime"
	"runtime/cgo"
	"sync"
	"unsafe"
)

func init() {
	C.sharedApplication()
}

// VirtualMachineState represents execution state of the virtual machine.
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

	// VZVirtualMachineStateStopping The virtual machine is being stopped.
	// This is the intermediate state between VZVirtualMachineStateRunning and VZVirtualMachineStateStop.
	VirtualMachineStateStopping
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

	pointer
	dispatchQueue unsafe.Pointer
	status        cgo.Handle

	mu sync.Mutex
}

type machineStatus struct {
	state       VirtualMachineState
	stateNotify chan VirtualMachineState

	mu sync.RWMutex
}

// NewVirtualMachine creates a new VirtualMachine with VirtualMachineConfiguration.
//
// The configuration must be valid. Validation can be performed at runtime with (*VirtualMachineConfiguration).Validate() method.
// The configuration is copied by the initializer.
//
// A new dispatch queue will create when called this function.
// Every operation on the virtual machine must be done on that queue. The callbacks and delegate methods are invoked on that queue.
func NewVirtualMachine(config *VirtualMachineConfiguration) *VirtualMachine {
	cs := getUUID()
	dispatchQueue := C.makeDispatchQueue(cs.CString())
	cs.Free()

	status := cgo.NewHandle(&machineStatus{
		state:       VirtualMachineState(0),
		stateNotify: make(chan VirtualMachineState),
	})

	v := &VirtualMachine{
		id: cs.String(),
		pointer: pointer{
			ptr: C.newVZVirtualMachineWithDispatchQueue(
				config.Ptr(),
				dispatchQueue,
				unsafe.Pointer(&status),
			),
		},
		dispatchQueue: dispatchQueue,
		status:        status,
	}

	runtime.SetFinalizer(v, func(self *VirtualMachine) {
		self.status.Delete()
		releaseDispatch(self.dispatchQueue)
		self.Release()
	})
	return v
}

// SocketDevices return the list of socket devices configured on this virtual machine.
// Return an empty array if no socket device is configured.
//
// Since only NewVirtioSocketDeviceConfiguration is available in vz package,
// it will always return VirtioSocketDevice.
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine/3656702-socketdevices?language=objc
func (v *VirtualMachine) SocketDevices() []*VirtioSocketDevice {
	nsArray := &NSArray{
		pointer: pointer{
			ptr: C.VZVirtualMachine_socketDevices(v.Ptr()),
		},
	}
	ptrs := nsArray.ToPointerSlice()
	socketDevices := make([]*VirtioSocketDevice, len(ptrs))
	for i, ptr := range ptrs {
		socketDevices[i] = newVirtioSocketDevice(ptr, v.dispatchQueue)
	}
	return socketDevices
}

//export changeStateOnObserver
func changeStateOnObserver(state C.int, cgoHandlerPtr unsafe.Pointer) {
	status := *(*cgo.Handle)(cgoHandlerPtr)
	// I expected it will not cause panic.
	// if caused panic, that's unexpected behavior.
	v, _ := status.Value().(*machineStatus)
	v.mu.Lock()
	newState := VirtualMachineState(state)
	v.state = newState
	// for non-blocking
	go func() { v.stateNotify <- newState }()
	v.mu.Unlock()
}

// State represents execution state of the virtual machine.
func (v *VirtualMachine) State() VirtualMachineState {
	// I expected it will not cause panic.
	// if caused panic, that's unexpected behavior.
	val, _ := v.status.Value().(*machineStatus)
	val.mu.RLock()
	defer val.mu.RUnlock()
	return val.state
}

// StateChangedNotify gets notification is changed execution state of the virtual machine.
func (v *VirtualMachine) StateChangedNotify() <-chan VirtualMachineState {
	// I expected it will not cause panic.
	// if caused panic, that's unexpected behavior.
	val, _ := v.status.Value().(*machineStatus)
	val.mu.RLock()
	defer val.mu.RUnlock()
	return val.stateNotify
}

// CanStart returns true if the machine is in a state that can be started.
func (v *VirtualMachine) CanStart() bool {
	return bool(C.vmCanStart(v.Ptr(), v.dispatchQueue))
}

// CanPause returns true if the machine is in a state that can be paused.
func (v *VirtualMachine) CanPause() bool {
	return bool(C.vmCanPause(v.Ptr(), v.dispatchQueue))
}

// CanResume returns true if the machine is in a state that can be resumed.
func (v *VirtualMachine) CanResume() bool {
	return (bool)(C.vmCanResume(v.Ptr(), v.dispatchQueue))
}

// CanRequestStop returns whether the machine is in a state where the guest can be asked to stop.
func (v *VirtualMachine) CanRequestStop() bool {
	return (bool)(C.vmCanRequestStop(v.Ptr(), v.dispatchQueue))
}

// CanStop returns whether the machine is in a state that can be stopped.
func (v *VirtualMachine) CanStop() bool {
	return (bool)(C.vmCanStop(v.Ptr(), v.dispatchQueue))
}

//export virtualMachineCompletionHandler
func virtualMachineCompletionHandler(cgoHandlerPtr, errPtr unsafe.Pointer) {
	cgoHandler := *(*cgo.Handle)(cgoHandlerPtr)

	handler := cgoHandler.Value().(func(error))

	if err := newNSError(errPtr); err != nil {
		handler(err)
	} else {
		handler(nil)
	}
}

func makeHandler(fn func(error)) (func(error), chan struct{}) {
	done := make(chan struct{})
	return func(err error) {
		fn(err)
		close(done)
	}, done
}

// Start a virtual machine that is in either Stopped or Error state.
//
// - fn parameter called after the virtual machine has been successfully started or on error.
// The error parameter passed to the block is null if the start was successful.
func (v *VirtualMachine) Start(fn func(error)) {
	h, done := makeHandler(fn)
	handler := cgo.NewHandle(h)
	defer handler.Delete()
	C.startWithCompletionHandler(v.Ptr(), v.dispatchQueue, unsafe.Pointer(&handler))
	<-done
}

// Pause a virtual machine that is in Running state.
//
// - fn parameter called after the virtual machine has been successfully paused or on error.
// The error parameter passed to the block is null if the pause was successful.
func (v *VirtualMachine) Pause(fn func(error)) {
	h, done := makeHandler(fn)
	handler := cgo.NewHandle(h)
	defer handler.Delete()
	C.pauseWithCompletionHandler(v.Ptr(), v.dispatchQueue, unsafe.Pointer(&handler))
	<-done
}

// Resume a virtual machine that is in the Paused state.
//
// - fn parameter called after the virtual machine has been successfully resumed or on error.
// The error parameter passed to the block is null if the resumption was successful.
func (v *VirtualMachine) Resume(fn func(error)) {
	h, done := makeHandler(fn)
	handler := cgo.NewHandle(h)
	defer handler.Delete()
	C.resumeWithCompletionHandler(v.Ptr(), v.dispatchQueue, unsafe.Pointer(&handler))
	<-done
}

// RequestStop requests that the guest turns itself off.
//
// If returned error is not nil, assigned with the error if the request failed.
// Returns true if the request was made successfully.
func (v *VirtualMachine) RequestStop() (bool, error) {
	nserr := newNSErrorAsNil()
	nserrPtr := nserr.Ptr()
	ret := (bool)(C.requestStopVirtualMachine(v.Ptr(), v.dispatchQueue, &nserrPtr))
	if err := newNSError(nserrPtr); err != nil {
		return ret, err
	}
	return ret, nil
}

// Stop stops a VM that’s in either a running or paused state.
//
// The completion handler returns an error object when the VM fails to stop,
// or nil if the stop was successful.
//
// Warning: This is a destructive operation. It stops the VM without
// giving the guest a chance to stop cleanly.
func (v *VirtualMachine) Stop(fn func(error)) {
	h, done := makeHandler(fn)
	handler := cgo.NewHandle(h)
	defer handler.Delete()
	C.stopWithCompletionHandler(v.Ptr(), v.dispatchQueue, unsafe.Pointer(&handler))
	<-done
}

// StartGraphicApplication starts an application to display graphics of the VM.
//
// You must to call runtime.LockOSThread before calling this method.
func (v *VirtualMachine) StartGraphicApplication(width, height float64) {
	C.startVirtualMachineWindow(v.Ptr(), C.double(width), C.double(height))
}
