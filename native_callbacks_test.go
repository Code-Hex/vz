//go:build darwin

package vz

import (
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestNativeRequestCompletesOnce(t *testing.T) {
	var registry nativeCallbackRegistry
	var calls atomic.Int32
	id := registry.add(func(uint32, unsafe.Pointer, unsafe.Pointer, uint64) uint64 { calls.Add(1); return 0 }, true)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if f, ok := registry.take(id, false); ok {
				f(1, nil, nil, 0)
			}
		}()
	}
	workers.Wait()
	if calls.Load() != 1 {
		t.Fatalf("completion count = %d", calls.Load())
	}
	next := registry.add(func(uint32, unsafe.Pointer, unsafe.Pointer, uint64) uint64 { return 0 }, true)
	if next == id {
		t.Fatal("reused a completed request identifier")
	}
}

func TestNativeSubscriptionEndsAtDestruction(t *testing.T) {
	var registry nativeCallbackRegistry
	id := registry.add(func(uint32, unsafe.Pointer, unsafe.Pointer, uint64) uint64 { return 0 }, false)
	for range 3 {
		if _, ok := registry.take(id, false); !ok {
			t.Fatal("event removed subscription")
		}
	}
	if _, ok := registry.take(id, true); !ok {
		t.Fatal("lost terminal event")
	}
	if _, ok := registry.take(id, false); ok {
		t.Fatal("accepted event after destruction")
	}
}
