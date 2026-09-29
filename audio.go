//go:build darwin

package vz

import (
	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

// AudioDeviceConfiguration interface for an audio device configuration.
type AudioDeviceConfiguration interface {
	objc.NSObject

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
	*pointer

	*baseAudioDeviceConfiguration
}

var _ AudioDeviceConfiguration = (*VirtioSoundDeviceConfiguration)(nil)

// NewVirtioSoundDeviceConfiguration creates a new sound device configuration.
//
// This is only supported on macOS 12 and newer, error will be returned
// on older versions.
func NewVirtioSoundDeviceConfiguration() (*VirtioSoundDeviceConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &VirtioSoundDeviceConfiguration{
		pointer: vzbridge.Framework_VZVirtioSoundDeviceConfiguration_init_6435e630(),
	}
	return config, nil
}

// SetStreams sets the list of audio streams exposed by this device.
func (v *VirtioSoundDeviceConfiguration) SetStreams(streams ...VirtioSoundDeviceStreamConfiguration) {
	array := nativeObjectArray(streams)
	vzbridge.SetSoundStreams(v, array)
}

// VirtioSoundDeviceStreamConfiguration interface for Virtio Sound Device Stream Configuration.
type VirtioSoundDeviceStreamConfiguration interface {
	objc.NSObject

	virtioSoundDeviceStreamConfiguration()
}

type baseVirtioSoundDeviceStreamConfiguration struct{}

func (*baseVirtioSoundDeviceStreamConfiguration) virtioSoundDeviceStreamConfiguration() {}

// VirtioSoundDeviceHostInputStreamConfiguration is a PCM stream of input audio data,
// such as from a microphone via host.
type VirtioSoundDeviceHostInputStreamConfiguration struct {
	*pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceHostInputStreamConfiguration)(nil)

// NewVirtioSoundDeviceHostInputStreamConfiguration creates a new PCM stream configuration of input audio data from host.
//
// This is only supported on macOS 12 and newer, error will be returned
// on older versions.
func NewVirtioSoundDeviceHostInputStreamConfiguration() (*VirtioSoundDeviceHostInputStreamConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &VirtioSoundDeviceHostInputStreamConfiguration{
		pointer: vzbridge.NewVirtioSoundDeviceHostInputStreamConfiguration(),
	}
	return config, nil
}

// VirtioSoundDeviceOutputStreamConfiguration defines a PCM output stream whose
// audio is suppressed by the host. Virtualization keeps the stream clocked while
// discarding the samples when no sink is configured.
type VirtioSoundDeviceOutputStreamConfiguration struct {
	*pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceOutputStreamConfiguration)(nil)

// NewVirtioSoundDeviceOutputStreamConfiguration creates a new sound device
// output stream configuration without a host audio sink.
//
// This is only supported on macOS 12 and newer, error will be returned
// on older versions.
func NewVirtioSoundDeviceOutputStreamConfiguration() (*VirtioSoundDeviceOutputStreamConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &VirtioSoundDeviceOutputStreamConfiguration{
		pointer: vzbridge.Framework_VZVirtioSoundDeviceOutputStreamConfiguration_init_225412a0(),
	}
	return config, nil
}

// VirtioSoundDeviceHostOutputStreamConfiguration is a struct that
// defines a Virtio host sound device output stream configuration.
//
// A PCM stream of output audio data, such as to a speaker from host.
type VirtioSoundDeviceHostOutputStreamConfiguration struct {
	*pointer

	*baseVirtioSoundDeviceStreamConfiguration
}

var _ VirtioSoundDeviceStreamConfiguration = (*VirtioSoundDeviceHostOutputStreamConfiguration)(nil)

// NewVirtioSoundDeviceHostOutputStreamConfiguration creates a new sounds device output stream configuration.
//
// This is only supported on macOS 12 and newer, error will be returned
// on older versions.
func NewVirtioSoundDeviceHostOutputStreamConfiguration() (*VirtioSoundDeviceHostOutputStreamConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &VirtioSoundDeviceHostOutputStreamConfiguration{
		pointer: vzbridge.NewVirtioSoundDeviceHostOutputStreamConfiguration(),
	}
	return config, nil
}
