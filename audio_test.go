//go:build darwin

package vz_test

import (
	"errors"
	"testing"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestVirtioSoundDeviceInputStreamWithoutSource(t *testing.T) {
	stream, err := vz.NewVirtioSoundDeviceInputStreamConfiguration()
	if errors.Is(err, vz.ErrUnsupportedOSVersion) {
		t.Skipf("not supported on this macOS version: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(stream)
	if objc.Ptr(stream) == nil {
		t.Fatal("input stream has no native object")
	}
	if source := vzbridge.VZVirtioSoundDeviceInputStreamConfiguration_Source(stream); source != nil {
		objc.Release(source)
		t.Fatal("input stream has a host audio source")
	}

	device, err := vz.NewVirtioSoundDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(device)
	device.SetStreams(stream)
	streams := vzbridge.VZVirtioSoundDeviceConfiguration_Streams(device)
	defer objc.Release(streams)
	if count := vzbridge.NSArray_Count(streams); count != 1 {
		t.Fatalf("audio stream count = %d, want 1", count)
	}
	input := vzbridge.NSArray_ObjectAtIndex(streams, 0)
	defer objc.Release(input)
	if objc.Ptr(input) != objc.Ptr(stream) {
		t.Fatal("audio device does not contain the input stream")
	}
}

func TestVirtioSoundDeviceOutputStreamWithoutSink(t *testing.T) {
	stream, err := vz.NewVirtioSoundDeviceOutputStreamConfiguration()
	if errors.Is(err, vz.ErrUnsupportedOSVersion) {
		t.Skipf("not supported on this macOS version: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(stream)
	if objc.Ptr(stream) == nil {
		t.Fatal("output stream has no native object")
	}
	if sink := vzbridge.VZVirtioSoundDeviceOutputStreamConfiguration_Sink(stream); sink != nil {
		objc.Release(sink)
		t.Fatal("output stream has a host audio sink")
	}
}
