//go:build darwin && arm64

package vz_test

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/objc"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestMacInputConfigurations(t *testing.T) {
	t.Run("keyboard", func(t *testing.T) {
		checkNativeClass(t, 14, vz.NewMacKeyboardConfiguration, "VZMacKeyboardConfiguration")
	})
	t.Run("trackpad", func(t *testing.T) {
		checkNativeClass(t, 13, vz.NewMacTrackpadConfiguration, "VZMacTrackpadConfiguration")
	})
}

func TestMacGraphicsConfigurationDisplays(t *testing.T) {
	if vz.Available(12) {
		t.Skip("requires macOS 12")
	}
	device, err := vz.NewMacGraphicsDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(device)
	display, err := vz.NewMacGraphicsDisplayConfiguration(1920, 1080, 144)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int64{"widthInPixels": 1920, "heightInPixels": 1080, "pixelsPerInch": 144} {
		if got := nativeProperty[int64](display, name); got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	device.SetDisplays(display)
	objc.Release(display)
	array := objc.NewNSArray(nativeProperty[unsafe.Pointer](device, "displays"))
	items := array.ToPointerSlice()
	if len(items) != 1 || nativeProperty[int64](objc.NewPointer(items[0]), "pixelsPerInch") != 144 {
		t.Fatal("display was not retained by the graphics configuration")
	}
	device.SetDisplays()
	if count := nativeProperty[pureobjc.ID](device, "displays").Send(pureobjc.RegisterName("count")); count != 0 {
		t.Fatalf("cleared displays count = %d", count)
	}
}
