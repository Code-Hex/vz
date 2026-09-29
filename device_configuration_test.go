//go:build darwin

package vz_test

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/objc"
	pureobjc "github.com/ebitengine/purego/objc"
)

func checkNativeClass[T objc.NSObject](t *testing.T, version float64, newConfig func() (T, error), class string) {
	t.Helper()
	if vz.Available(version) {
		t.Skipf("requires macOS %.1f", version)
	}
	config, err := newConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(config)
	id := pureobjc.ID(uintptr(objc.Ptr(config)))
	if id == 0 || id.Class() != pureobjc.GetClass(class) {
		t.Fatalf("constructor did not create %s", class)
	}
}

func nativeProperty[T any](object objc.NSObject, name string) T {
	value := pureobjc.Send[T](pureobjc.ID(uintptr(objc.Ptr(object))), pureobjc.RegisterName(name))
	runtime.KeepAlive(object)
	return value
}

func TestDeviceConfigurations(t *testing.T) {
	t.Run("entropy", func(t *testing.T) {
		checkNativeClass(t, 11, vz.NewVirtioEntropyDeviceConfiguration, "VZVirtioEntropyDeviceConfiguration")
	})
	t.Run("keyboard", func(t *testing.T) {
		checkNativeClass(t, 12, vz.NewUSBKeyboardConfiguration, "VZUSBKeyboardConfiguration")
	})
	t.Run("pointing", func(t *testing.T) {
		checkNativeClass(t, 12, vz.NewUSBScreenCoordinatePointingDeviceConfiguration, "VZUSBScreenCoordinatePointingDeviceConfiguration")
	})
}

func TestAudioConfigurationStreams(t *testing.T) {
	if vz.Available(12) {
		t.Skip("requires macOS 12")
	}
	device, err := vz.NewVirtioSoundDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(device)
	input, err := vz.NewVirtioSoundDeviceHostInputStreamConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	output, err := vz.NewVirtioSoundDeviceHostOutputStreamConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	source := nativeProperty[pureobjc.ID](input, "source")
	sink := nativeProperty[pureobjc.ID](output, "sink")
	if source == 0 || source.Class() != pureobjc.GetClass("VZHostAudioInputStreamSource") {
		t.Fatal("input has no host audio source")
	}
	if sink == 0 || sink.Class() != pureobjc.GetClass("VZHostAudioOutputStreamSink") {
		t.Fatal("output has no host audio sink")
	}
	device.SetStreams(input, output)
	inputPtr, outputPtr := objc.Ptr(input), objc.Ptr(output)
	objc.Release(input)
	objc.Release(output)
	array := objc.NewNSArray(nativeProperty[unsafe.Pointer](device, "streams"))
	items := array.ToPointerSlice()
	if len(items) != 2 || items[0] != inputPtr || items[1] != outputPtr {
		t.Fatalf("audio streams = %v, want input then output", items)
	}
	device.SetStreams()
	if count := nativeProperty[pureobjc.ID](device, "streams").Send(pureobjc.RegisterName("count")); count != 0 {
		t.Fatalf("cleared streams count = %d", count)
	}
}

func TestGraphicsConfigurationScanouts(t *testing.T) {
	if vz.Available(13) {
		t.Skip("requires macOS 13")
	}
	device, err := vz.NewVirtioGraphicsDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(device)
	scanout, err := vz.NewVirtioGraphicsScanoutConfiguration(1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := nativeProperty[int64](scanout, "widthInPixels"), nativeProperty[int64](scanout, "heightInPixels"); w != 1280 || h != 720 {
		t.Fatalf("scanout dimensions = %dx%d, want 1280x720", w, h)
	}
	device.SetScanouts(scanout)
	objc.Release(scanout)
	array := objc.NewNSArray(nativeProperty[unsafe.Pointer](device, "scanouts"))
	items := array.ToPointerSlice()
	if len(items) != 1 || nativeProperty[int64](objc.NewPointer(items[0]), "widthInPixels") != 1280 {
		t.Fatal("scanout was not retained by the graphics configuration")
	}
	device.SetScanouts()
	if count := nativeProperty[pureobjc.ID](device, "scanouts").Send(pureobjc.RegisterName("count")); count != 0 {
		t.Fatalf("cleared scanouts count = %d", count)
	}
}
