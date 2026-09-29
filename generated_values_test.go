//go:build darwin

package vz

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestGeneratedValueMigration(t *testing.T) {
	for _, tt := range []struct {
		selector string
		got      uint64
	}{
		{"minimumAllowedMemorySize", VirtualMachineConfigurationMinimumAllowedMemorySize()},
		{"maximumAllowedMemorySize", VirtualMachineConfigurationMaximumAllowedMemorySize()},
		{"minimumAllowedCPUCount", uint64(VirtualMachineConfigurationMinimumAllowedCPUCount())},
		{"maximumAllowedCPUCount", uint64(VirtualMachineConfigurationMaximumAllowedCPUCount())},
	} {
		want := pureobjc.Send[uint64](pureobjc.ID(pureobjc.GetClass("VZVirtualMachineConfiguration")), pureobjc.RegisterName(tt.selector))
		if tt.got != want {
			t.Errorf("%s: got %d, want %d", tt.selector, tt.got, want)
		}
	}
	if macOSAvailable(13) == nil {
		for _, tt := range []struct {
			name, class, selector string
			call                  func() (string, error)
		}{
			{"automount", "VZVirtioFileSystemDeviceConfiguration", "macOSGuestAutomountTag", MacOSGuestAutomountTag},
			{"clipboard", "VZSpiceAgentPortAttachment", "spiceAgentPortName", SpiceAgentPortAttachmentName},
		} {
			got, err := tt.call()
			if err != nil {
				t.Fatal(err)
			}
			if want := nativeClassString(tt.class, tt.selector); got != want {
				t.Errorf("%s: got %q, want %q", tt.name, got, want)
			}
		}
	}
	if macOSAvailable(15) == nil {
		want := pureobjc.Send[bool](pureobjc.ID(pureobjc.GetClass("VZGenericPlatformConfiguration")), pureobjc.RegisterName("isNestedVirtualizationSupported"))
		if got := IsNestedVirtualizationSupported(); got != want {
			t.Errorf("nested virtualization: got %v, want %v", got, want)
		}
	}
}

func nativeClassString(class, selector string) string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.NewObject("NSAutoreleasePool")
	defer objc.Release(pool)
	value := pureobjc.Send[pureobjc.ID](pureobjc.ID(pureobjc.GetClass(class)), pureobjc.RegisterName(selector))
	return pureobjc.Send[string](value, pureobjc.RegisterName("UTF8String"))
}
