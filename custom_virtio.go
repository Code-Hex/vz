package vz

import (
	"context"
	"errors"
	"runtime"
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/ebitengine/purego"
)

// CustomVirtioContext gives a delegate access to its parent context and the
// device's serial queue. A delegate may retain it for work that finishes after
// a callback returns. Retaining the context also keeps the native device alive,
// but does not extend the lifetime of guest memory or queue elements.
//
// Deadline, Done, Err, and Value forward to the context supplied through
// WithCustomVirtioContext, or context.Background if that option is omitted.
// Device lifecycle events and provider closure do not cancel the parent.
// Parent cancellation does not stop the virtual machine or close the provider.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegateprovider/devicequeue?language=objc
type CustomVirtioContext interface {
	context.Context

	// RunOnDeviceQueue runs fn on the device's serial queue, waits for fn to
	// return, and returns its error unchanged. Calls from the same queue run
	// fn directly, so fn may call other device and queue methods.
	//
	// Delegate callbacks already run on this queue, and individual device and
	// queue methods dispatch to it as well. Use RunOnDeviceQueue to group your
	// own state checks with those operations.
	//
	// A backend worker can use RunOnDeviceQueue to finish a request after
	// doing blocking I/O outside the queue. It can check whether the request is
	// still valid, write the response, and return the queue element in one call.
	// Lifecycle callbacks cannot run between those steps.
	//
	// For example, a delegate can keep per-device fields d.generation and
	// d.stopped, accessed only on the device queue. WillReset increments
	// d.generation, and WillStop sets d.stopped. The notification callback
	// records requestGeneration from d.generation when it dequeues element,
	// before passing the request to a worker. After preparing response, the
	// worker's completion function can return:
	//
	//	return ctx.RunOnDeviceQueue(func() error {
	//		if d.stopped || d.generation != requestGeneration {
	//			return nil
	//		}
	//		if err := element.WriteBytes(response); err != nil {
	//			return err
	//		}
	//		return element.ReturnToQueue()
	//	})
	//
	// This discards a result if the delegate has observed a stop or reset.
	// Serializing host operations does not prevent the guest from changing
	// its memory while fn runs.
	//
	// fn must not wait for a worker or completion handler that needs this
	// queue. That would deadlock. RunOnDeviceQueue waits only for fn, not for
	// asynchronous operations started by fn. Configuration updates and shared
	// memory mappings complete on the device queue, so any wait for their
	// completion must happen after RunOnDeviceQueue returns, outside the queue.
	//
	// RunOnDeviceQueue calls fn even after parent cancellation, allowing
	// cleanup to run. The caller decides whether to check Err inside fn.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegateprovider/devicequeue?language=objc
	RunOnDeviceQueue(fn func() error) error
}

type customVirtioContext struct {
	context.Context
	device *CustomVirtioDevice
}

func (c *customVirtioContext) RunOnDeviceQueue(fn func() error) error {
	var err error
	vzbridge.OnCustomVirtioQueue(c.device, func() {
		err = fn()
	})
	return err
}

// CustomVirtioDeviceDelegate implements a custom device on its serial device queue.
// Methods must not wait for virtual machine operations, which may themselves wait
// for the delegate, or workers that need the device queue. Queue notifications
// must drain NextElement until it returns nil.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate?language=objc
type CustomVirtioDeviceDelegate interface {
	// DidCreateDevice is called after the framework creates the device.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfigurationdelegate/customvirtioconfiguration(_:didcreatedevice:)?language=objc
	DidCreateDevice(CustomVirtioContext, *CustomVirtioDevice)

	// DidReceiveNotificationForQueue is called when the guest notifies a virtqueue.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevice(_:didreceivenotificationfor:)?language=objc
	DidReceiveNotificationForQueue(CustomVirtioContext, *CustomVirtioDevice, *VirtioQueue)

	// DidAcceptDriverOK is called after the device and driver complete Virtio negotiation.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevicedidacceptdriverok(_:)?language=objc
	DidAcceptDriverOK(CustomVirtioContext, *CustomVirtioDevice)

	// WillStop is called before the device stops.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevicewillstop(_:)?language=objc
	WillStop(CustomVirtioContext, *CustomVirtioDevice)

	// WillPause is called before the device pauses.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevicewillpause(_:)?language=objc
	WillPause(CustomVirtioContext, *CustomVirtioDevice)

	// WillResume is called before the device resumes.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevicewillresume(_:)?language=objc
	WillResume(CustomVirtioContext, *CustomVirtioDevice)

	// WillReset is called after the device reset completes.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevicewillreset(_:)?language=objc
	WillReset(CustomVirtioContext, *CustomVirtioDevice)
}

// CustomVirtioDeviceStateDelegate adds save and restore support to a delegate.
// A nil saved state indicates failure; use an empty non-nil slice for no state.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate?language=objc
type CustomVirtioDeviceStateDelegate interface {
	// SaveStateForRestore returns the device state to save with the virtual machine.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodevicesavestate(forrestore:)?language=objc
	SaveStateForRestore(CustomVirtioContext, *CustomVirtioDevice) []byte

	// ShouldRestore restores saved device state and reports whether restoration succeeded.
	//
	// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegate/customvirtiodeviceshouldrestore(_:savestate:)?language=objc
	ShouldRestore(CustomVirtioContext, *CustomVirtioDevice, []byte) bool
}

// CustomVirtioDeviceDelegateProvider runs a delegate on a dedicated serial queue.
// Call Close after the virtual machines using the provider have stopped.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegateprovider?language=objc
type CustomVirtioDeviceDelegateProvider struct {
	*pointer
	supportsSaveRestore bool
}

type customVirtioProviderOptions struct {
	parent context.Context
}

// CustomVirtioDeviceDelegateProviderOption configures a delegate provider.
type CustomVirtioDeviceDelegateProviderOption func(*customVirtioProviderOptions) error

// WithCustomVirtioContext sets the parent of each CustomVirtioContext passed to
// the delegate. The provider retains ctx for callbacks after construction.
// Without this option, the parent is context.Background. A nil ctx is an error.
//
// Canceling ctx closes the delegate context's Done channel. It does not stop
// the virtual machine, close the provider, or prevent RunOnDeviceQueue calls.
func WithCustomVirtioContext(ctx context.Context) CustomVirtioDeviceDelegateProviderOption {
	return func(options *customVirtioProviderOptions) error {
		if ctx == nil {
			return errors.New("custom Virtio parent context is nil")
		}
		options.parent = ctx
		return nil
	}
}

type customVirtioDelegateState struct {
	delegate CustomVirtioDeviceDelegate
	parent   context.Context
}

// NewCustomVirtioDeviceDelegateProvider creates a provider for delegate.
// Each callback receives a CustomVirtioContext with access to the device queue.
// Its parent defaults to context.Background. WithCustomVirtioContext sets a
// parent for work done by the delegate, including work after a callback returns.
// This requires macOS 27 or newer.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegateprovider/init(devicequeue:delegate:)?language=objc
func NewCustomVirtioDeviceDelegateProvider(delegate CustomVirtioDeviceDelegate, options ...CustomVirtioDeviceDelegateProviderOption) (*CustomVirtioDeviceDelegateProvider, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	if delegate == nil {
		return nil, errors.New("custom Virtio delegate is nil")
	}
	config := customVirtioProviderOptions{
		parent: context.Background(),
	}
	for _, option := range options {
		if err := option(&config); err != nil {
			return nil, err
		}
	}
	state := customVirtioDelegateState{
		delegate: delegate,
		parent:   config.parent,
	}
	handle := cgo.NewHandle(state)
	native := vzbridge.NewCustomVirtioProvider(customVirtioCallback(), uint64(handle))
	if native == nil || objc.Ptr(native) == nil {
		handle.Delete()
		return nil, errors.New("cannot create custom Virtio provider")
	}
	_, stateful := delegate.(CustomVirtioDeviceStateDelegate)
	return &CustomVirtioDeviceDelegateProvider{
		pointer:             native,
		supportsSaveRestore: stateful,
	}, nil
}

// Close unregisters the delegate and waits for any running callback to finish.
// It does not cancel or wait for application workers, or invalidate retained
// CustomVirtioContext values. Guest resources remain subject to the VM lifecycle.
// It is safe to call repeatedly or from a delegate method.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevicedelegateprovider/delegate?language=objc
func (p *CustomVirtioDeviceDelegateProvider) Close() error {
	vzbridge.CloseCustomVirtioProvider(p)
	return nil
}

// WithCustomVirtioProvider sets the device implementation.
// Save and restore support is enabled when its delegate implements
// CustomVirtioDeviceStateDelegate.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration/provider?language=objc
func WithCustomVirtioProvider(provider *CustomVirtioDeviceDelegateProvider) CustomVirtioDeviceConfigurationOption {
	return func(c *CustomVirtioDeviceConfiguration) error {
		var native objc.NSObject
		if provider != nil {
			native = provider
		}
		vzbridge.VZCustomVirtioDeviceConfiguration_SetProvider(c, native)
		vzbridge.VZCustomVirtioDeviceConfiguration_SetSupportsSaveRestore(c, provider != nil && provider.supportsSaveRestore)
		return nil
	}
}

// CustomVirtioDevice is a running custom Virtio device supplied to its delegate.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice?language=objc
type CustomVirtioDevice struct {
	*pointer
}

var customVirtioCallbackInit sync.Once
var customVirtioCallbackPointer uintptr

func customVirtioCallback() uintptr {
	customVirtioCallbackInit.Do(func() {
		customVirtioCallbackPointer = purego.NewCallback(dispatchCustomVirtioCallback)
	})
	return customVirtioCallbackPointer
}

func dispatchCustomVirtioCallback(event uint32, context uint64, first, second unsafe.Pointer) uintptr {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handle := cgo.Handle(context)
	if event == vzbridge.CustomVirtioReleased {
		handle.Delete()
		return 0
	}
	if event == vzbridge.CustomVirtioCompleted {
		completion := handle.Value().(func(error))
		handle.Delete()
		var err error
		if first != nil {
			err = objcutil.NewNSError(first)
		}
		completion(err)
		return 0
	}
	state := handle.Value().(customVirtioDelegateState)
	delegate := state.delegate
	device := &CustomVirtioDevice{
		pointer: objc.NewManagedPointer(first, vzbridge.ReleaseObject),
	}
	ctx := &customVirtioContext{
		Context: state.parent,
		device:  device,
	}
	switch event {
	case vzbridge.CustomVirtioCreated:
		delegate.DidCreateDevice(ctx, device)
	case vzbridge.CustomVirtioNotified:
		delegate.DidReceiveNotificationForQueue(ctx, device, &VirtioQueue{
			pointer: objc.NewManagedPointer(second, vzbridge.ReleaseObject),
		})
	case vzbridge.CustomVirtioDriverReady:
		delegate.DidAcceptDriverOK(ctx, device)
	case vzbridge.CustomVirtioWillStop:
		delegate.WillStop(ctx, device)
	case vzbridge.CustomVirtioWillPause:
		delegate.WillPause(ctx, device)
	case vzbridge.CustomVirtioWillResume:
		delegate.WillResume(ctx, device)
	case vzbridge.CustomVirtioWillReset:
		delegate.WillReset(ctx, device)
	case vzbridge.CustomVirtioSave:
		if stateful, ok := delegate.(CustomVirtioDeviceStateDelegate); ok {
			data := stateful.SaveStateForRestore(ctx, device)
			if data == nil {
				return 0
			}
			native := vzbridge.NSData_DataWithBytes_Length(unsafe.Pointer(unsafe.SliceData(data)), uint64(len(data)))
			runtime.KeepAlive(data)
			return vzbridge.TransferCustomVirtioData(native)
		}
	case vzbridge.CustomVirtioRestore:
		data := objcutil.Bytes(objc.NewManagedPointer(second, vzbridge.ReleaseObject))
		if stateful, ok := delegate.(CustomVirtioDeviceStateDelegate); ok && stateful.ShouldRestore(ctx, device, data) {
			return 1
		}
	}
	return 0
}

// NegotiatedFeatures returns the features accepted by the guest, or nil before DRIVER_OK.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice/negotiatedfeatures?language=objc
func (d *CustomVirtioDevice) NegotiatedFeatures() *VirtioFeatureSet {
	native := vzbridge.VZCustomVirtioDevice_NegotiatedFeatures(d)
	if native == nil {
		return nil
	}
	defer objc.Release(native)
	return &VirtioFeatureSet{
		Subset0: vzbridge.VZNegotiatedVirtioFeatureSet_Subset0(native),
		Subset1: vzbridge.VZNegotiatedVirtioFeatureSet_Subset1(native),
	}
}

// QueueAtIndex returns a queue after DRIVER_OK, or nil if disabled or out of range.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice/queue(at:)?language=objc
func (d *CustomVirtioDevice) QueueAtIndex(index uint16) *VirtioQueue {
	native := vzbridge.VZCustomVirtioDevice_QueueAtIndex(d, index)
	if native == nil {
		return nil
	}
	return &VirtioQueue{pointer: native}
}

// RequestDeviceReset asks the guest to reset this device. The guest may ignore it.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice/requestreset()?language=objc
func (d *CustomVirtioDevice) RequestDeviceReset() {
	vzbridge.VZCustomVirtioDevice_RequestDeviceReset(d)
}

// GuestMemoryMapping returns a mapping of guest RAM, or nil for an invalid range.
// The mapping must not be used after the virtual machine reboots or shuts down.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice/guestmemorymapping(atphysicaladdress:length:)?language=objc
func (d *CustomVirtioDevice) GuestMemoryMapping(physicalAddress, length uint64) *GuestMemoryMapping {
	native := vzbridge.VZCustomVirtioDevice_GuestMemoryMappingAtPhysicalAddress_Length(d, physicalAddress, length)
	if native == nil {
		return nil
	}
	return &GuestMemoryMapping{pointer: native}
}

// SharedMemoryRegions returns the regions configured for the device.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice/sharedmemoryregions?language=objc
func (d *CustomVirtioDevice) SharedMemoryRegions() []*VirtioSharedMemoryRegion {
	array := vzbridge.VZCustomVirtioDevice_SharedMemoryRegions(d)
	objects := nativeArray(array)
	regions := make([]*VirtioSharedMemoryRegion, len(objects))
	for i, object := range objects {
		regions[i] = &VirtioSharedMemoryRegion{pointer: object}
	}
	return regions
}

// UpdateDeviceSpecificConfiguration updates the device configuration asynchronously.
// The new configuration must have the same byte length as the original.
// completion runs on the device queue and must not block it.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice/update(_:completionhandler:)?language=objc
func (d *CustomVirtioDevice) UpdateDeviceSpecificConfiguration(configuration *VirtioDeviceSpecificConfiguration, completion func(error)) error {
	if configuration == nil || completion == nil {
		return errors.New("configuration and completion must not be nil")
	}
	if uint64(len(configuration.ConfigurationData())) != vzbridge.CustomVirtioConfigurationLength(d) {
		return errors.New("device configuration length cannot change")
	}
	handle := cgo.NewHandle(completion)
	vzbridge.UpdateCustomVirtioConfiguration(d, configuration, customVirtioCallback(), uint64(handle))
	return nil
}
