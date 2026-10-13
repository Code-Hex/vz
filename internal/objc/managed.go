//go:build darwin

package objc

import (
	"runtime"
	"sync"
	"unsafe"
)

type managedOwner struct {
	reference *managedReference
}

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
	owner := &managedOwner{reference: state}
	runtime.AddCleanup(owner, (*managedReference).close, state)
	return &Pointer{managed: owner}
}

type managedPointerInitializer interface {
	initializeManagedPointer(*Pointer)
}

// InitializeManagedPointer shares source's owner with a fresh, unpublished destination.
func InitializeManagedPointer(destination managedPointerInitializer, source *Pointer) {
	destination.initializeManagedPointer(source)
}

func (p *Pointer) initializeManagedPointer(source *Pointer) {
	if source == nil || source.managed == nil {
		panic("objc: managed pointer source is nil or uninitialized")
	}
	if p.managed != nil {
		panic("objc: managed pointer destination is already initialized")
	}
	*p = *source
}
