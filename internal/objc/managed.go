//go:build darwin

package objc

import (
	"runtime"
	"sync"
	"unsafe"
)

type managedReference struct {
	once    sync.Once
	pointer unsafe.Pointer
	release func(unsafe.Pointer)
}

func (r *managedReference) close() {
	r.once.Do(func() { r.release(r.pointer) })
}

// NewManagedPointer adopts one native reference shared by every Go alias.
func NewManagedPointer(pointer unsafe.Pointer, release func(unsafe.Pointer)) *Pointer {
	if pointer == nil {
		return nil
	}
	state := &managedReference{pointer: pointer, release: release}
	p := &Pointer{_ptr: pointer, managed: state}
	runtime.AddCleanup(p, (*managedReference).close, state)
	return p
}
