//go:build darwin

package vz

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestNativeStartedMachineCollectionReleasesSubscriptions(t *testing.T) {
	if err := macOSAvailable(12); err != nil {
		t.Skip("stopping a virtual machine requires macOS 12")
	}
	runtime.GC()
	vzbridge.Drain()
	before := nativeCallbacks.next.Load()
	stop := stoppedNativeMachineStates(t)
	awaitNativeBridge(t, func() bool {
		return nativeBridgeCallbackCountAfter(before) == 0
	})
	stop()
}

func stoppedNativeMachineStates(t *testing.T) func() {
	t.Helper()
	loader, err := NewLinuxBootLoader("testdata/Image", WithInitrd("testdata/initramfs.cpio.gz"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := NewVirtualMachineConfiguration(loader, 1, 256*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if machine.CanStop() {
			if err := machine.Stop(); err != nil {
				t.Error(err)
			}
		}
	}()
	if err := machine.Start(); err != nil {
		t.Fatal(err)
	}
	if err := machine.Stop(); err != nil {
		t.Fatal(err)
	}
	if machine.State() != VirtualMachineStateStopped {
		t.Fatalf("state = %s", machine.State())
	}
	stop, err := machine.StateChangedNotify(make(chan VirtualMachineState, 1))
	if err != nil {
		t.Fatal(err)
	}
	return stop
}
