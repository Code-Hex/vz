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

type VirtioSoundDeviceConfiguration struct {
	pointer

	*baseAudioDeviceConfiguration
}

var _ AudioDeviceConfiguration = (*VirtioSoundDeviceConfiguration)(nil)

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

type VirtioSoundDeviceHostInputStreamConfiguration struct {
	pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceHostInputStreamConfiguration)(nil)

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

type VirtioSoundDeviceHostOutputStreamConfiguration struct {
	pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceHostOutputStreamConfiguration)(nil)

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
