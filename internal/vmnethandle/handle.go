package vmnethandle

import "github.com/Code-Hex/vz/v4/internal/objc"

// Handle shares one managed Core Foundation reference between Go aliases.
type Handle struct {
	pointer *objc.Pointer
}

// New adopts a managed Core Foundation owner.
func New(pointer *objc.Pointer) Handle {
	return Handle{pointer: pointer}
}

func (h *Handle) nativePointer() *objc.Pointer {
	return h.pointer
}

func (h *Handle) adoptRetainedNetwork(pointer *objc.Pointer) {
	h.pointer = pointer
}

// Pointer borrows the managed owner without transferring its native reference.
func Pointer(owner interface{ nativePointer() *objc.Pointer }) *objc.Pointer {
	return owner.nativePointer()
}

// AdoptRetainedNetwork initializes an empty owner with one managed retained network reference.
func AdoptRetainedNetwork(owner interface{ adoptRetainedNetwork(*objc.Pointer) }, pointer *objc.Pointer) {
	owner.adoptRetainedNetwork(pointer)
}
