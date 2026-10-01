//go:build darwin

package vz

import (
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

func TestNativeStartedMachineCollectionClosesSubscriptions(t *testing.T) {
	if err := macOSAvailable(12); err != nil {
		t.Skip("stopping a virtual machine requires macOS 12")
	}
	runtime.GC()
	vzbridge.Drain()
	before := nativeCallbacks.next.Load()
	states := stoppedNativeMachineStates(t)
	closed := false
	defer func() {
		if t.Failed() {
			t.Logf("stopped VM subscription closed=%t, remaining callbacks=%d", closed, nativeBridgeCallbackCountAfter(before))
		}
	}()
	awaitNativeBridge(t, func() bool {
		for !closed {
			select {
			case _, open := <-states:
				closed = !open
			default:
				return false
			}
		}
		return nativeBridgeCallbackCountAfter(before) == 0
	})
}

func stoppedNativeMachineStates(t *testing.T) <-chan VirtualMachineState {
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
	states := machine.StateChangedNotify()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case state, open := <-states:
			if !open {
				t.Fatal("machine subscription closed before the stopped notification")
			}
			if state == VirtualMachineStateStopped {
				return states
			}
		case <-deadline.C:
			t.Fatalf("machine did not report stopped, current state is %s", machine.State())
		}
	}
}
