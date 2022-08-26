package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

// AudioDeviceConfiguration interface for an audio device configuration.
type AudioDeviceConfiguration interface {
	NSObject

	audioDeviceConfiguration()
}

type baseAudioDeviceConfiguration struct{}

func (*baseAudioDeviceConfiguration) audioDeviceConfiguration() {}

// VirtioSoundDeviceConfiguration is a struct that defines a Virtio sound device configuration.
//
// Use a VirtioSoundDeviceConfiguration to configure an audio device for your VM. After creating
// this struct, assign appropriate values via the SetStreams method which defines the behaviors of
// the underlying audio streams for this audio device.
//
// After creating and configuring a VirtioSoundDeviceConfiguration struct, assign it to the
// SetAudioDevicesVirtualMachineConfiguration method of your VM’s configuration.
type VirtioSoundDeviceConfiguration struct {
	pointer

	*baseAudioDeviceConfiguration
}

var _ AudioDeviceConfiguration = (*VirtioSoundDeviceConfiguration)(nil)

// NewVirtioSoundDeviceConfiguration creates a new sound device configuration.
func NewVirtioSoundDeviceConfiguration() *VirtioSoundDeviceConfiguration {
	config := &VirtioSoundDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioSoundDeviceConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *VirtioSoundDeviceConfiguration) {
		self.Release()
	})
	return config
}

// SetStreams sets the list of audio streams exposed by this device.
func (v *VirtioSoundDeviceConfiguration) SetStreams(streams ...VirtioSoundDeviceStreamConfiguration) {
	ptrs := make([]NSObject, len(streams))
	for i, val := range streams {
		ptrs[i] = val
	}
	array := convertToNSMutableArray(ptrs)
	C.setStreamsVZVirtioSoundDeviceConfiguration(v.Ptr(), array.Ptr())
}

// VirtioSoundDeviceStreamConfiguration interface for Virtio Sound Device Stream Configuration.
type VirtioSoundDeviceStreamConfiguration interface {
	NSObject

	virtioSoundDeviceStreamConfiguration()
}

type baseVirtioSoundDeviceStreamConfiguration struct{}

func (*baseVirtioSoundDeviceStreamConfiguration) virtioSoundDeviceStreamConfiguration() {}

// VirtioSoundDeviceHostInputStreamConfiguration is a PCM stream of input audio data,
// such as from a microphone via host.
type VirtioSoundDeviceHostInputStreamConfiguration struct {
	pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceHostInputStreamConfiguration)(nil)

// NewVirtioSoundDeviceHostInputStreamConfiguration creates a new PCM stream configuration of input audio data from host.
func NewVirtioSoundDeviceHostInputStreamConfiguration() *VirtioSoundDeviceHostInputStreamConfiguration {
	config := &VirtioSoundDeviceHostInputStreamConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioSoundDeviceHostInputStreamConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *VirtioSoundDeviceHostInputStreamConfiguration) {
		self.Release()
	})
	return config
}

// VirtioSoundDeviceHostOutputStreamConfiguration is a struct that
// defines a Virtio host sound device output stream configuration.
//
// A PCM stream of output audio data, such as to a speaker from host.
type VirtioSoundDeviceHostOutputStreamConfiguration struct {
	pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceHostOutputStreamConfiguration)(nil)

// NewVirtioSoundDeviceHostOutputStreamConfiguration creates a new sounds device output stream configuration.
func NewVirtioSoundDeviceHostOutputStreamConfiguration() *VirtioSoundDeviceHostOutputStreamConfiguration {
	config := &VirtioSoundDeviceHostOutputStreamConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioSoundDeviceHostOutputStreamConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *VirtioSoundDeviceHostOutputStreamConfiguration) {
		self.Release()
	})
	return config
}
