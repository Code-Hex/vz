//go:build darwin

package vz_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
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

func assertGoPanic(t *testing.T, call func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("nil wrapper did not panic")
		} else if _, ok := recovered.(runtime.Error); !ok {
			t.Fatalf("panic = %v, want a Go runtime error", recovered)
		}
	}()
	call()
}

func TestNilDeviceConfigurationElements(t *testing.T) {
	config, err := vz.NewVirtualMachineConfiguration(nil, 1, 512*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		call func()
	}{
		{"entropy", func() {
			config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{nil})
		}},
		{"serial", func() {
			config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{nil})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { assertGoPanic(t, tc.call) })
	}
	t.Run("scanout", func(t *testing.T) {
		if vz.Available(13) {
			t.Skip("requires macOS 13")
		}
		device, err := vz.NewVirtioGraphicsDeviceConfiguration()
		if err != nil {
			t.Fatal(err)
		}
		assertGoPanic(t, func() { device.SetScanouts(nil) })
	})
}

func TestDeviceConfigurations(t *testing.T) {
	t.Run("entropy", func(t *testing.T) {
		checkNativeClass(t, 11, func() (*objc.Pointer, error) {
			config, err := vz.NewVirtioEntropyDeviceConfiguration()
			return (*objc.Pointer)(config), err
		}, "VZVirtioEntropyDeviceConfiguration")
	})
	t.Run("keyboard", func(t *testing.T) {
		checkNativeClass(t, 12, vz.NewUSBKeyboardConfiguration, "VZUSBKeyboardConfiguration")
	})
	t.Run("pointing", func(t *testing.T) {
		checkNativeClass(t, 12, vz.NewUSBScreenCoordinatePointingDeviceConfiguration, "VZUSBScreenCoordinatePointingDeviceConfiguration")
	})
}

func TestVirtualMachineConfigurationRetainsEntropyAndSerialPorts(t *testing.T) {
	file, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	attachment, err := vz.NewFileHandleSerialPortAttachment(file, file)
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtualMachineConfiguration(nil, 1, 512*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(config)
	entropy, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	serial, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{entropy})
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{serial})
	entropyPtr, serialPtr := objc.Ptr((*objc.Pointer)(entropy)), objc.Ptr((*objc.Pointer)(serial))
	objc.Release((*objc.Pointer)(entropy))
	objc.Release((*objc.Pointer)(serial))
	for _, tc := range []struct {
		property string
		want     pureobjc.ID
		class    string
	}{
		{"entropyDevices", pureobjc.ID(uintptr(entropyPtr)), "VZVirtioEntropyDeviceConfiguration"},
		{"serialPorts", pureobjc.ID(uintptr(serialPtr)), "VZVirtioConsoleDeviceSerialPortConfiguration"},
	} {
		array := nativeProperty[pureobjc.ID](config, tc.property)
		if count := array.Send(pureobjc.RegisterName("count")); count != 1 {
			t.Fatalf("%s count = %d, want 1", tc.property, count)
		}
		item := pureobjc.Send[pureobjc.ID](array, pureobjc.RegisterName("objectAtIndex:"), uintptr(0))
		if item != tc.want || item.Class() != pureobjc.GetClass(tc.class) {
			t.Fatalf("%s did not retain the supplied configuration", tc.property)
		}
		if tc.property == "serialPorts" {
			if got := pureobjc.Send[pureobjc.ID](item, pureobjc.RegisterName("attachment")); got != pureobjc.ID(uintptr(objc.Ptr(attachment))) {
				t.Fatal("serial port attachment differs")
			}
		}
	}
	config.SetEntropyDevicesVirtualMachineConfiguration(nil)
	config.SetSerialPortsVirtualMachineConfiguration(nil)
	for _, property := range []string{"entropyDevices", "serialPorts"} {
		if count := nativeProperty[pureobjc.ID](config, property).Send(pureobjc.RegisterName("count")); count != 0 {
			t.Fatalf("cleared %s count = %d, want 0", property, count)
		}
	}
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
	array := vzbridge.VZVirtioSoundDeviceConfiguration_Streams(device)
	if count := vzbridge.NSArray_Count(array); count != 2 {
		t.Fatalf("audio stream count = %d, want 2", count)
	}
	first := vzbridge.NSArray_ObjectAtIndex(array, 0)
	second := vzbridge.NSArray_ObjectAtIndex(array, 1)
	if objc.Ptr(first) != inputPtr || objc.Ptr(second) != outputPtr {
		t.Fatal("audio streams are not input then output")
	}
	objc.Release(first)
	objc.Release(second)
	objc.Release(array)
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
	if w, h := nativeProperty[int64]((*objc.Pointer)(scanout), "widthInPixels"), nativeProperty[int64]((*objc.Pointer)(scanout), "heightInPixels"); w != 1280 || h != 720 {
		t.Fatalf("scanout dimensions = %dx%d, want 1280x720", w, h)
	}
	device.SetScanouts(scanout)
	objc.Release((*objc.Pointer)(scanout))
	array := vzbridge.VZVirtioGraphicsDeviceConfiguration_Scanouts(device)
	if count := vzbridge.NSArray_Count(array); count != 1 {
		t.Fatalf("scanout count = %d, want 1", count)
	}
	item := vzbridge.NSArray_ObjectAtIndex(array, 0)
	if nativeProperty[int64](item, "widthInPixels") != 1280 {
		t.Fatal("scanout was not retained by the graphics configuration")
	}
	objc.Release(item)
	objc.Release(array)
	device.SetScanouts()
	if count := nativeProperty[pureobjc.ID](device, "scanouts").Send(pureobjc.RegisterName("count")); count != 0 {
		t.Fatalf("cleared scanouts count = %d", count)
	}
}
