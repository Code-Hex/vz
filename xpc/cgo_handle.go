package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"runtime"
	"runtime/cgo"
)

// cgoHandler holds a cgo.Handle for an Object.
// It provides methods to hold and release the handle.
// handle will released when cgoHandler.release is called.
type cgoHandler struct {
	handle cgo.Handle
}

// releaseOnCleanup registers a cleanup function to delete the cgo.Handle when cleaned up.
func (h *cgoHandler) releaseOnCleanup() {
	runtime.AddCleanup(h, func(h cgo.Handle) {
		h.Delete()
	}, h.handle)
}

// newCgoHandler creates a new cgoHandler and holds the given value.
func newCgoHandler(v any) (*cgoHandler, C.uintptr_t) {
	if v == nil {
		return nil, 0
	}
	h := &cgoHandler{cgo.NewHandle(v)}
	return ReleaseOnCleanup(h), C.uintptr_t(h.handle)
}

// unwrapHandler unwraps the cgo.Handle from the given uintptr and returns the associated value.
// It does NOT delete the handle; it expects the handle to be managed by cgoHandler or caller.
func unwrapHandler[T any](handle uintptr) T {
	if handle == 0 {
		var zero T
		return zero
	}
	return cgo.Handle(handle).Value().(T)
}
