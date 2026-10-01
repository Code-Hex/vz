//go:build darwin && debug

package vz

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

func TestDebugConfigurationBridge(t *testing.T) {
	debug, err := NewGDBDebugStubConfiguration(54321)
	if err != nil {
		t.Fatal(err)
	}
	config := newTestConfig(t)
	config.SetDebugStubVirtualMachineConfiguration(debug)
	port := vzbridge.UnsafePrivate__VZGDBDebugStubConfiguration_Instance_port_21ffbdf5(objc.Ptr(debug))
	if port != 54321 {
		t.Fatalf("debug port = %d, want 54321", port)
	}
	runtime.KeepAlive(debug)
	runtime.KeepAlive(config)
}
