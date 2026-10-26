//go:build darwin

package vz

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// USBAccessory is a USB accessory made available by Apple's AccessoryAccess framework.
// Obtain accessories through USBAccessoryChangedNotify.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessory?language=objc
type USBAccessory struct {
	*pointer
}

// RegistryID returns the accessory's IORegistry ID.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessory/registryid?language=objc
func (a *USBAccessory) RegistryID() uint64 {
	return vzbridge.USBAccessoryRegistryID(a)
}

// DeviceDescriptorData returns a copy of the USB device descriptor.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessory/devicedescriptordata?language=objc
func (a *USBAccessory) DeviceDescriptorData() []byte {
	data := vzbridge.USBAccessoryDeviceDescriptor(a)
	return objcutil.Bytes(data)
}

// ConfigurationDescriptorData returns a copy of the current USB configuration
// descriptor, or nil when the accessory has no selected configuration.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessory/configurationdescriptordata?language=objc
func (a *USBAccessory) ConfigurationDescriptorData() []byte {
	data := vzbridge.USBAccessoryConfigurationDescriptor(a)
	return objcutil.Bytes(data)
}

// USBAccessoryEvent reports an accessory connection or disconnection.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessorylistener?language=objc
type USBAccessoryEvent struct {
	// Accessory is the accessory that connected or disconnected.
	Accessory *USBAccessory
	// Connected is true for a connection and false for a disconnection.
	Connected bool
}

// USBAccessoryChangedNotify registers for accessory connections and disconnections,
// and returns the accessories already connected when registration completes.
// It requires macOS 27 and an ordinary application that appears in the Dock,
// signed with the com.apple.developer.accessory-access.usb entitlement.
// AccessoryAccess lets the user choose which accessories the application can use.
//
// Delivery to channel does not block. Events are discarded when channel cannot
// receive them. Call stop before closing channel. Calling stop more than once is safe.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessorymanager?language=objc
func USBAccessoryChangedNotify(channel chan<- USBAccessoryEvent) (accessories []*USBAccessory, stop func(), err error) {
	if err := macOSAvailable(27); err != nil {
		return nil, nil, err
	}
	events := new(notifications[USBAccessoryEvent])
	if _, err := events.subscribe(channel); err != nil {
		return nil, nil, err
	}
	type registrationResult struct {
		accessories []*USBAccessory
		err         error
	}
	registered := make(chan registrationResult, 1)
	handle := registerNativeCallback(func(kind uint32, first, second unsafe.Pointer, _ uint64) uint64 {
		switch kind {
		case vzbridge.CallbackAccessoryRegistered:
			array := objc.NewManagedPointer(first, vzbridge.ReleaseObject)
			var accessories []*USBAccessory
			if array != nil {
				for _, ptr := range nativeArray(array) {
					accessory := &USBAccessory{
						pointer: ptr,
					}
					accessories = append(accessories, accessory)
				}
			}
			result := registrationResult{accessories: accessories}
			if err := objcutil.NewNSError(second); err != nil {
				result.err = err
			}
			registered <- result
		case vzbridge.CallbackAccessoryConnected, vzbridge.CallbackAccessoryDisconnected:
			events.send(USBAccessoryEvent{
				Accessory: &USBAccessory{
					pointer: objc.NewManagedPointer(first, vzbridge.ReleaseObject),
				},
				Connected: kind == vzbridge.CallbackAccessoryConnected,
			})
		case vzbridge.CallbackContextReleased:
			events.close()
		}
		return 0
	})
	listener := vzbridge.RegisterUSBAccessories(handle)
	result := <-registered
	if result.err != nil {
		events.close()
		unregisterNativeCallback(handle)
		if listener != nil {
			objc.Release(listener)
		}
		return nil, nil, result.err
	}
	var once sync.Once
	stop = func() {
		once.Do(func() {
			events.close()
			completion, done := nativeCompletion()
			vzbridge.UnregisterUSBAccessories(listener, completion)
			<-done
			unregisterNativeCallback(handle)
			objc.Release(listener)
		})
	}
	return result.accessories, stop, nil
}

// USBPassthroughDeviceConfiguration configures a USB accessory for a virtual machine.
// The accessory is captured when the VM starts with this configuration or
// when USBController.Attach attaches the device to a running VM.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbpassthroughdeviceconfiguration?language=objc
type USBPassthroughDeviceConfiguration struct {
	*pointer
}

func (*USBPassthroughDeviceConfiguration) usbDeviceConfiguration() {}

// NewUSBPassthroughDeviceConfiguration creates a configuration for accessory.
// This requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbpassthroughdeviceconfiguration/init(device:)?language=objc
func NewUSBPassthroughDeviceConfiguration(accessory *USBAccessory, options ...USBDeviceConfigurationOption) (*USBPassthroughDeviceConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	if accessory == nil || objc.Ptr(accessory) == nil {
		return nil, errors.New("vz: nil USB accessory")
	}
	configuration := &USBPassthroughDeviceConfiguration{
		pointer: vzbridge.VZUSBPassthroughDeviceConfiguration_InitWithDevice(accessory),
	}
	for _, option := range options {
		if err := option(configuration); err != nil {
			objc.Release(configuration)
			return nil, err
		}
	}
	return configuration, nil
}

// NewUSBPassthroughDevice creates a device that can be attached with USBController.Attach.
// This requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbpassthroughdevice?language=objc
func NewUSBPassthroughDevice(configuration *USBPassthroughDeviceConfiguration) (USBDevice, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	if configuration == nil || objc.Ptr(configuration) == nil {
		return nil, errors.New("vz: nil USB passthrough configuration")
	}
	var nserr unsafe.Pointer
	ptr := vzbridge.VZUSBPassthroughDevice_InitWithConfiguration_Error(configuration, &nserr)
	if err := objcutil.NewNSError(nserr); err != nil {
		return nil, err
	}
	return &usbDevice{
		pointer: ptr,
	}, nil
}

// USBPassthroughDeviceDidDisconnectNotify reports passthrough devices whose host USB service terminated.
// The framework has already detached the device when it delivers the notification.
// This requires macOS 27 or later. Only one registration per controller is allowed.
//
// Delivery does not block. Notifications are discarded when channel cannot receive.
// Call stop before closing channel. Calling stop more than once is safe.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbcontroller/delegate-swift.protocol/usbcontroller(_:usbpassthroughdevicediddisconnect:)?language=objc
func (u *USBController) USBPassthroughDeviceDidDisconnectNotify(channel chan<- USBDevice) (stop func(), err error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	events := new(notifications[USBDevice])
	if _, err := events.subscribe(channel); err != nil {
		return nil, err
	}
	handle := registerNativeCallback(func(kind uint32, first, _ unsafe.Pointer, _ uint64) uint64 {
		switch kind {
		case vzbridge.CallbackUSBPassthroughDisconnected:
			device := &usbDevice{
				pointer: objc.NewManagedPointer(first, vzbridge.ReleaseObject),
			}
			events.send(device)
		case vzbridge.CallbackContextReleased:
			events.close()
		}
		return 0
	})
	var nserr unsafe.Pointer
	observer := vzbridge.ObserveUSBDisconnect(u, handle, &nserr)
	if err := objcutil.NewNSError(nserr); err != nil {
		unregisterNativeCallback(handle)
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			events.close()
			vzbridge.StopObservingUSBDisconnect(u, observer)
			unregisterNativeCallback(handle)
			objc.Release(observer)
			runtime.KeepAlive(u)
		})
	}, nil
}
