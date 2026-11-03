//go:build darwin

package vz_test

import (
	"errors"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestGenericPlatformConfigurationNestedVirtualizationEnabled(t *testing.T) {
	if vz.Available(12) {
		t.Skip("generic platform configuration requires macOS 12 or later")
	}
	configuration, err := vz.NewGenericPlatformConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := configuration.IsNestedVirtualizationEnabled()
	if vz.Available(15) {
		if enabled || (!errors.Is(err, vz.ErrUnsupportedOSVersion) && !errors.Is(err, vz.ErrBuildTargetOSVersion)) {
			t.Fatalf("got (%v, %v), want false and an unsupported-version error", enabled, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("nested virtualization is enabled by default")
	}

	if vz.IsNestedVirtualizationSupported() {
		if err := configuration.SetNestedVirtualizationEnabled(true); err != nil {
			t.Fatal(err)
		}
		enabled, err = configuration.IsNestedVirtualizationEnabled()
		if err != nil {
			t.Fatal(err)
		}
		if !enabled {
			t.Fatal("nested virtualization is disabled after enabling it")
		}
	}
	if err := configuration.SetNestedVirtualizationEnabled(false); err != nil {
		t.Fatal(err)
	}
	enabled, err = configuration.IsNestedVirtualizationEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("nested virtualization is enabled after disabling it")
	}
}
