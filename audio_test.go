//go:build darwin

package vz_test

import (
	"errors"
	"testing"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

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
