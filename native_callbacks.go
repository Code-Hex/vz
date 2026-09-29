//go:build darwin

package vz

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/vzbridge"
	"github.com/ebitengine/purego"
)

type nativeCallback func(uint32, unsafe.Pointer, unsafe.Pointer, uint64) uint64

type nativeCallbackEntry struct {
	callback nativeCallback
	once     bool
}

type nativeCallbackRegistry struct {
	next    atomic.Uint64
	mu      sync.Mutex
	entries map[uint64]nativeCallbackEntry
}

func (r *nativeCallbackRegistry) add(callback nativeCallback, once bool) uint64 {
	id := r.next.Add(1)
	if id == 0 {
		panic("native callback identifiers exhausted")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[uint64]nativeCallbackEntry)
	}
	r.entries[id] = nativeCallbackEntry{callback: callback, once: once}
	return id
}

func (r *nativeCallbackRegistry) take(id uint64, terminal bool) (nativeCallback, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id]
	if ok && (terminal || entry.once) {
		delete(r.entries, id)
	}
	return entry.callback, ok
}

func (r *nativeCallbackRegistry) remove(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, id)
}

var nativeCallbacks nativeCallbackRegistry
var nativeCallbackInit sync.Once

func initializeNativeCallbacks() {
	nativeCallbackInit.Do(func() {
		vzbridge.SetCallback(uint64(purego.NewCallback(dispatchNativeCallback)))
	})
}

func registerNativeCallback(callback nativeCallback) uint64 {
	initializeNativeCallbacks()
	return nativeCallbacks.add(callback, false)
}

func registerNativeRequest(callback nativeCallback) uint64 {
	initializeNativeCallbacks()
	return nativeCallbacks.add(callback, true)
}

func unregisterNativeCallback(id uint64) { nativeCallbacks.remove(id) }

func dispatchNativeCallback(kind uint32, context uint64, first, second unsafe.Pointer, value uint64) uint64 {
	if callback, ok := nativeCallbacks.take(context, kind == 4); ok {
		return callback(kind, first, second, value)
	}
	switch kind {
	case 1, 3, 5, 6, 7, 9, 10, 11:
		if first != nil {
			vzbridge.ReleaseObject(first)
		}
	}
	if (kind == 5 || kind == 10) && second != nil {
		vzbridge.ReleaseObject(second)
	}
	return 0
}
