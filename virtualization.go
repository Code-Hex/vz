package vz

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/sliceutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// IsSupported reports whether the host supports hardware virtualization.
// It returns false on macOS versions older than 11.
func IsSupported() bool {
	if macOSAvailable(11) != nil {
		return false
	}
	return vzbridge.VZVirtualMachine_IsSupported()
}

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
	*pointer
	machineState *machineState

	disconnected *notifications[*DisconnectedError]

	config *VirtualMachineConfiguration
}

type machineState struct {
	mu               sync.RWMutex
	state            VirtualMachineState
	stateNotify      notifications[VirtualMachineState]
	guestStops       notifications[struct{}]
	stopErrors       notifications[error]
	consolePorts     notifications[ConsolePortEvent]
	graphicsDisplays notifications[GraphicsDisplayEvent]
}

func (s *machineState) close() {
	s.stateNotify.close()
	s.guestStops.close()
	s.stopErrors.close()
	s.consolePorts.close()
	s.graphicsDisplays.close()
}

// NewVirtualMachine creates a new VirtualMachine with VirtualMachineConfiguration.
//
// The configuration must be valid. Validation can be performed at runtime with (*VirtualMachineConfiguration).Validate() method.
// The configuration is copied by the initializer.
//
// Each virtual machine has its own serial dispatch queue.
// vz dispatches VM operations to that queue internally.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtualMachine(config *VirtualMachineConfiguration) (*VirtualMachine, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}
	state := &machineState{}
	stateHandle := registerNativeCallback(func(kind uint32, errorPointer, _ unsafe.Pointer, value uint64) uint64 {
		switch kind {
		case vzbridge.CallbackGuestStopped:
			state.guestStops.send(struct{}{})
			return 0
		case vzbridge.CallbackConsolePortOpened, vzbridge.CallbackConsolePortClosed:
			state.consolePorts.send(ConsolePortEvent{
				DeviceIndex: int(value >> 32),
				PortIndex:   uint32(value),
				Open:        kind == vzbridge.CallbackConsolePortOpened,
			})
			return 0
		case vzbridge.CallbackDisplayReconfigurationBegan, vzbridge.CallbackDisplayReconfigurationEnded:
			state.graphicsDisplays.send(GraphicsDisplayEvent{
				DeviceIndex:   int(value >> 32),
				DisplayIndex:  int(uint32(value)),
				Reconfiguring: kind == vzbridge.CallbackDisplayReconfigurationBegan,
			})
			return 0
		case vzbridge.CallbackMachineStoppedWithError:
			err := objcutil.NewNSError(errorPointer)
			state.stopErrors.send(err)
			return 0
		case vzbridge.CallbackContextReleased:
			state.close()
			return 0
		}
		state.mu.Lock()
		state.state = VirtualMachineState(value)
		state.stateNotify.send(state.state)
		state.mu.Unlock()
		return 0
	})
	disconnected := &notifications[*DisconnectedError]{}
	configs := config.networkDeviceConfiguration
	disconnectedHandle := registerNativeCallback(func(kind uint32, errorPointer, _ unsafe.Pointer, value uint64) uint64 {
		if kind == vzbridge.CallbackContextReleased {
			disconnected.close()
			return 0
		}
		disconnected.send(&DisconnectedError{
			Err:    objcutil.NewNSError(errorPointer),
			Config: sliceutil.FindValueByIndex(configs, int(value)),
		})
		return 0
	})
	object := vzbridge.NewVZVirtualMachineWithDispatchQueue(config, stateHandle, disconnectedHandle)
	if object == nil {
		unregisterNativeCallback(stateHandle)
		unregisterNativeCallback(disconnectedHandle)
		state.close()
		disconnected.close()
		return nil, fmt.Errorf("could not create virtual machine")
	}
	return &VirtualMachine{
		pointer:      object,
		machineState: state,
		disconnected: disconnected,
		config:       config,
	}, nil
}

// SocketDevices return the list of socket devices configured on this virtual machine.
// Return an empty array if no socket device is configured.
//
// Since only NewVirtioSocketDeviceConfiguration is available in vz package,
// it will always return VirtioSocketDevice.
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine/3656702-socketdevices?language=objc
func (v *VirtualMachine) SocketDevices() []*VirtioSocketDevice {

	pointers := nativeArray(vzbridge.VZVirtualMachine_SocketDevices(v))
	values := make([]*VirtioSocketDevice, len(pointers))
	for i, ptr := range pointers {
		values[i] = &VirtioSocketDevice{vm: v, pointer: ptr}
	}
	return values
}

// USBControllers return the list of USB controllers configured on this virtual machine. Return an empty array if no USB controller is configured.
//
// This is only supported on macOS 15 and newer, nil will
// be returned on older versions.
func (v *VirtualMachine) USBControllers() []*USBController {
	if macOSAvailable(15) != nil {
		return nil
	}
	pointers := nativeArray(vzbridge.VZVirtualMachine_UsbControllers(v))
	values := make([]*USBController, len(pointers))
	for i, ptr := range pointers {
		values[i] = &USBController{vm: v, pointer: ptr}
	}
	return values
}

// State represents execution state of the virtual machine.
func (v *VirtualMachine) State() VirtualMachineState {
	v.machineState.mu.RLock()
	defer v.machineState.mu.RUnlock()
	return v.machineState.state
}

// GuestDidStopNotify registers c for notifications when the guest stops the VM,
// including in response to RequestStop. Host Stop calls and failures do not notify c.
// Delivery follows StateChangedNotify. The returned function unregisters c.
func (v *VirtualMachine) GuestDidStopNotify(c chan<- struct{}) (func(), error) {
	return v.machineState.guestStops.subscribe(c)
}

// DidStopWithErrorNotify registers c for errors that caused the VM to stop.
// Normal guest shutdown and successful host Stop calls do not notify c.
// Delivery follows StateChangedNotify. The returned function unregisters c.
func (v *VirtualMachine) DidStopWithErrorNotify(c chan<- error) (func(), error) {
	return v.machineState.stopErrors.subscribe(c)
}

// StateChangedNotify registers c for future execution state changes.
// Delivery never waits: a notification is dropped if c cannot receive it.
// The caller chooses the channel capacity. No past notifications are replayed.
//
// To wait for a state, register a dedicated channel with capacity at least one,
// then check State() before waiting and after each notification. The notification
// value may be out of date, and intermediate states may be missed.
//
// Registering the same channel again does not duplicate delivery. Either returned
// stop function removes that registration. The stop function is safe to call more than once;
// after it returns, that registration will not send again. Stop every registration
// using c before closing it. The library never closes c, including on VM release.
// A nil channel or a released VM returns an error and a nil stop function.
// Keep the VM reachable while receiving notifications.
func (v *VirtualMachine) StateChangedNotify(c chan<- VirtualMachineState) (func(), error) {
	return v.machineState.stateNotify.subscribe(c)
}

// CanStart returns true if the machine is in a state that can be started.
func (v *VirtualMachine) CanStart() bool {
	return bool(vzbridge.VZVirtualMachine_CanStart(v))
}

// CanPause returns true if the machine is in a state that can be paused.
func (v *VirtualMachine) CanPause() bool {
	return bool(vzbridge.VZVirtualMachine_CanPause(v))
}

// CanResume returns true if the machine is in a state that can be resumed.
func (v *VirtualMachine) CanResume() bool {
	return (bool)(vzbridge.VZVirtualMachine_CanResume(v))
}

// CanRequestStop returns whether the machine is in a state where the guest can be asked to stop.
func (v *VirtualMachine) CanRequestStop() bool {
	return (bool)(vzbridge.VZVirtualMachine_CanRequestStop(v))
}

// CanStop returns whether the machine is in a state that can be stopped.
//
// This is only supported on macOS 12 and newer, false will always be returned
// on older versions.
func (v *VirtualMachine) CanStop() bool {
	if err := macOSAvailable(12); err != nil {
		return false
	}
	return (bool)(vzbridge.VZVirtualMachine_CanStop(v))
}

type virtualMachineStartOptions struct {
	macOSVirtualMachineStartOptionsPtr *pointer
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

	handle, errCh := nativeCompletion()

	if o.macOSVirtualMachineStartOptionsPtr != nil {
		vzbridge.StartWithOptionsCompletionHandler(
			v,
			o.macOSVirtualMachineStartOptionsPtr,
			handle,
		)
	} else {
		vzbridge.StartWithCompletionHandler(v, handle)
	}
	return <-errCh
}

// Pause a virtual machine that is in Running state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
func (v *VirtualMachine) Pause() error {
	handle, errCh := nativeCompletion()
	vzbridge.PauseWithCompletionHandler(v, handle)
	return <-errCh
}

// Resume a virtual machine that is in the Paused state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
func (v *VirtualMachine) Resume() error {
	handle, errCh := nativeCompletion()
	vzbridge.ResumeWithCompletionHandler(v, handle)
	return <-errCh
}

// RequestStop requests that the guest turns itself off.
//
// If returned error is not nil, assigned with the error if the request failed.
// Returns true if the request was made successfully.
func (v *VirtualMachine) RequestStop() (bool, error) {
	var nserrPtr unsafe.Pointer
	ret := (bool)(vzbridge.VZVirtualMachine_RequestStopWithError(v, &nserrPtr))
	if err := objcutil.NewNSError(nserrPtr); err != nil {
		return ret, err
	}
	return ret, nil
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
	handle, errCh := nativeCompletion()
	vzbridge.StopWithCompletionHandler(v, handle)
	return <-errCh
}

type startGraphicApplicationOptions struct {
	title            string
	enableController bool
}

// StartGraphicApplicationOption is an option for display graphics start.
type StartGraphicApplicationOption func(*startGraphicApplicationOptions) error

// WithWindowTitle is an option to set window title of display graphics window.
func WithWindowTitle(title string) StartGraphicApplicationOption {
	return func(sgao *startGraphicApplicationOptions) error {
		sgao.title = title
		return nil
	}
}

// WithController is an option to set virtual machine controller on graphics window toolbar.
func WithController(enable bool) StartGraphicApplicationOption {
	return func(sgao *startGraphicApplicationOptions) error {
		sgao.enableController = enable
		return nil
	}
}

// GraphicWindow describes a VM window. Create one with NewGraphicWindow.
type GraphicWindow struct {
	machine *VirtualMachine
	width   float64
	height  float64
	options startGraphicApplicationOptions
}

// NewGraphicWindow configures a window without opening it.
func NewGraphicWindow(machine *VirtualMachine, width, height float64, opts ...StartGraphicApplicationOption) (*GraphicWindow, error) {
	if machine == nil {
		return nil, fmt.Errorf("graphic window requires a virtual machine")
	}
	if width <= 0 || height <= 0 || math.IsNaN(width) || math.IsNaN(height) || math.IsInf(width, 0) || math.IsInf(height, 0) {
		return nil, fmt.Errorf("graphic window dimensions must be finite and positive")
	}
	window := &GraphicWindow{
		machine: machine,
		width:   width,
		height:  height,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("graphic window option must not be nil")
		}
		if err := opt(&window.options); err != nil {
			return nil, err
		}
	}
	return window, nil
}

// StartGraphicApplication opens one or more VM windows.
// Closing a window does not stop its VM. A stopped VM closes its own window.
//
// The first call must run on the main goroutine after runtime.LockOSThread in
// init. It runs the event loop and returns when all windows close. While the
// event loop is running, calls from any goroutine add windows and return once
// they are displayed. A VM can have only one open window.
// Calls made while the application is shutting down return an error.
// After the event loop exits, a new application must start on the main thread.
// This is supported on macOS 12 and newer.
func StartGraphicApplication(w *GraphicWindow, windows ...*GraphicWindow) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	windows = append([]*GraphicWindow{w}, windows...)
	seen := make(map[*VirtualMachine]bool, len(windows))
	for _, window := range windows {
		if window == nil || window.machine == nil {
			return fmt.Errorf("graphic application requires valid windows")
		}
		if seen[window.machine] {
			return fmt.Errorf("graphic application cannot display the same virtual machine twice")
		}
		seen[window.machine] = true
	}
	application := vzbridge.NewGraphicApplication()
	defer vzbridge.FreeGraphicApplication(application)
	for _, window := range windows {
		vzbridge.AddGraphicWindow(application, window.machine, window.width, window.height, window.options.title, window.options.enableController)
	}
	status := vzbridge.RunGraphicApplication(application)
	runtime.KeepAlive(windows)
	switch status {
	case vzbridge.GraphicApplicationNotMainThread:
		return fmt.Errorf("graphic application must run on the main thread")
	case vzbridge.GraphicApplicationStopping:
		return fmt.Errorf("graphic application is shutting down")
	case vzbridge.GraphicApplicationDuplicateMachine:
		return fmt.Errorf("graphic application cannot display the same virtual machine twice")
	}
	return nil
}

// StartGraphicApplication opens a window for this VM.
//
// It follows the same threading and return behavior as the package-level
// StartGraphicApplication function.
// This is supported on macOS 12 and newer.
func (v *VirtualMachine) StartGraphicApplication(width, height float64, opts ...StartGraphicApplicationOption) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	window, err := NewGraphicWindow(v, width, height, opts...)
	if err != nil {
		return err
	}
	return StartGraphicApplication(window)
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

// NetworkDeviceAttachmentWasDisconnected registers c for network attachment
// disconnections, such as when a network interface fails to start. This can happen
// during boot, a device reset, or a reboot, and may happen more than once.
// The framework clears the affected runtime device's attachment when it disconnects.
// Delivery follows StateChangedNotify. The returned function unregisters c.
// This requires macOS 12 or newer.
func (v *VirtualMachine) NetworkDeviceAttachmentWasDisconnected(c chan<- *DisconnectedError) (func(), error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	return v.disconnected.subscribe(c)
}

func nativeCompletion() (uint64, <-chan error) {
	channel := make(chan error, 1)
	handle := registerNativeRequest(func(_ uint32, errorPointer, _ unsafe.Pointer, _ uint64) uint64 {
		if err := objcutil.NewNSError(errorPointer); err != nil {
			channel <- err
		} else {
			channel <- nil
		}
		close(channel)
		return 0
	})
	return handle, channel
}
