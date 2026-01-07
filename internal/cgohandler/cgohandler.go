package cgohandler

import (
	"runtime"
	"runtime/cgo"
)

// Handler holds a cgo.Handle for an Object.
// It provides methods to hold and release the handle.
// handle will released when Handler is cleaned up.
type Handler struct {
	handle cgo.Handle
}

// releaseOnCleanup registers a cleanup function to delete the cgo.Handle when cleaned up.
func (h *Handler) releaseOnCleanup() {
	runtime.AddCleanup(h, func(h cgo.Handle) {
		h.Delete()
	}, h.handle)
}

// New creates a new [Handler] and holds the given value.
func New(v any) (*Handler, uintptr) {
	if v == nil {
		return nil, 0
	}
	h := &Handler{cgo.NewHandle(v)}
	h.releaseOnCleanup()
	return h, uintptr(h.handle)
}

// Unwrap unwraps the cgo.Handle from the given uintptr and returns the associated value.
// It does NOT delete the handle; it expects the handle to be managed by [Handler] or caller.
func Unwrap[T any](handle uintptr) T {
	if handle == 0 {
		var zero T
		return zero
	}
	return cgo.Handle(handle).Value().(T)
}
