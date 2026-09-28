package vmnet

/*
#include "vmnet_darwin.h"
*/
import "C"
import (
	"fmt"
	"runtime"
	"runtime/cgo"
	"sync"
	"unsafe"
)

// PacketsAvailableEventCallback receives the estimated number of packets ready to read.
type PacketsAvailableEventCallback func(estimatedCount int)

type interfaceCallbackState struct {
	mu      sync.Mutex
	iface   unsafe.Pointer
	queue   unsafe.Pointer
	handle  cgo.Handle
	stopped bool
}

//export callPacketsAvailableEventCallback
func callPacketsAvailableEventCallback(handle C.uintptr_t, estimatedCount C.int) {
	cgo.Handle(handle).Value().(PacketsAvailableEventCallback)(int(estimatedCount))
}

//export releasePacketsAvailableEventCallback
func releasePacketsAvailableEventCallback(handle C.uintptr_t) {
	cgo.Handle(handle).Delete()
}

// SetPacketsAvailableEventCallback registers one callback for packet availability.
// Pass nil to remove it. A callback may call this method to remove itself.
// Call Stop or remove the callback when it captures the Interface, so it can be collected.
func (i *Interface) SetPacketsAvailableEventCallback(callback PacketsAvailableEventCallback) error {
	if i == nil || i.callbackState == nil {
		return fmt.Errorf("interface is nil")
	}
	state := i.callbackState
	state.mu.Lock()
	defer state.mu.Unlock()
	defer runtime.KeepAlive(i)
	if state.stopped {
		return fmt.Errorf("interface is stopped")
	}
	if callback == nil {
		return state.clearLocked()
	}
	if state.queue != nil {
		return fmt.Errorf("packets available callback is already set")
	}
	handle := cgo.NewHandle(callback)
	var status C.uint32_t
	queue := C.VmnetSetPacketsAvailableEventCallback(state.iface, C.uintptr_t(handle), &status)
	if result := Return(status); result != ErrSuccess {
		handle.Delete()
		return fmt.Errorf("set packets available callback: %w", result)
	}
	state.queue = queue
	state.handle = handle
	return nil
}

func (state *interfaceCallbackState) clearLocked() error {
	if state.queue == nil {
		return nil
	}
	result := Return(C.VmnetClearPacketsAvailableEventCallback(state.iface, state.queue, C.uintptr_t(state.handle)))
	if result != ErrSuccess {
		return fmt.Errorf("clear packets available callback: %w", result)
	}
	state.queue = nil
	state.handle = 0
	return nil
}

func (state *interfaceCallbackState) stop() error {
	state.mu.Lock()
	if state.stopped {
		state.mu.Unlock()
		return fmt.Errorf("interface is stopped")
	}
	if err := state.clearLocked(); err != nil {
		state.mu.Unlock()
		return err
	}
	state.stopped = true
	state.mu.Unlock()

	result := Return(C.VmnetStopInterface(state.iface))
	if result != ErrSuccess {
		state.mu.Lock()
		state.stopped = false
		state.mu.Unlock()
		return fmt.Errorf("stop vmnet interface: %w", result)
	}
	return nil
}

func (state *interfaceCallbackState) cleanup() {
	state.mu.Lock()
	defer state.mu.Unlock()
	_ = state.clearLocked()
}
