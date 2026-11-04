//go:build darwin

package vz

import (
	"context"
	"errors"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestVirtualMachineContextCanceledBeforeSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var machine *VirtualMachine
	for _, operation := range []func() error{
		func() error { return machine.start(ctx) },
		func() error { return machine.stop(ctx) },
	} {
		if err := operation(); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	}
}

func TestNativeCompletionCancellationUnregistersRequest(t *testing.T) {
	handle, completed := nativeCompletion()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitNativeCompletion(ctx, handle, completed); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	nativeCallbacks.mu.Lock()
	_, registered := nativeCallbacks.entries[handle]
	nativeCallbacks.mu.Unlock()
	if registered {
		t.Fatal("canceled completion is still registered")
	}
	dispatchNativeCallback(vzbridge.CallbackCompletion, handle, nil, nil, 0)
}

func TestNativeCompletionClaimedBeforeCancellation(t *testing.T) {
	handle, completed := nativeCompletion()
	callback, ok := nativeCallbacks.take(handle, false)
	if !ok {
		t.Fatal("completion callback is missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitNativeCompletion(ctx, handle, completed); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	callback(vzbridge.CallbackCompletion, nil, nil, 0)
	if err := <-completed; err != nil {
		t.Fatalf("late completion = %v, want nil", err)
	}
}
