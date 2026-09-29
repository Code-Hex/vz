//go:build darwin && debug

package vz

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestDebugConfigurationBridge(t *testing.T) {
	debug, err := NewGDBDebugStubConfiguration(54321)
	if err != nil {
		t.Fatal(err)
	}
	config := newTestConfig(t)
	config.SetDebugStubVirtualMachineConfiguration(debug)
	port := pureobjc.Send[int64](pureobjc.ID(uintptr(objc.Ptr(debug))), pureobjc.RegisterName("port"))
	if port != 54321 {
		t.Fatalf("debug port = %d, want 54321", port)
	}
	runtime.KeepAlive(debug)
	runtime.KeepAlive(config)
}
