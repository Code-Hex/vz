//go:build darwin

package vz

import "testing"

func TestVirtualizationSupported(t *testing.T) {
	if _, err := NewVirtualMachine(newTestConfig(t)); err != nil {
		t.Fatalf("could not create a virtual machine: %v", err)
	}
	if !IsSupported() {
		t.Fatal("virtualization reported unsupported after creating a virtual machine")
	}
}

func TestVirtioDisplayPixelDensity(t *testing.T) {
	if macOSAvailable(14) != nil {
		t.Skip("requires macOS 14")
	}
	display, _ := retainedGraphicsDisplay(t)
	if _, err := display.PixelsPerInch(); err == nil {
		t.Fatal("virtio display reported a Mac pixel density")
	}
}
