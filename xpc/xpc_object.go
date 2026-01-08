package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

type pointer = objc.Pointer

// xpcObject wraps an XPC object ([xpc_object_t]).
// It is expected to be embedded in other structs as pointer to provide common functionality.
//
// [xpc_object_t]: https://developer.apple.com/documentation/xpc/xpc_object_t?language=objc
type xpcObject struct {
	*pointer
}

var _ Object = (*xpcObject)(nil)

// newXpcObject creates a new [xpcObject] from an existing xpc_object_t.
func newXpcObject(ptr unsafe.Pointer) *xpcObject {
	return &xpcObject{objc.NewPointer(ptr)}
}

// String returns the description of the [xpcObject].
//   - https://developer.apple.com/documentation/xpc/xpc_copy_description(_:)?language=objc
func (x *xpcObject) String() string {
	cs := C.xpcCopyDescription(objc.Ptr(x))
	defer C.free(unsafe.Pointer(cs))
	return C.GoString(cs)
}

// retain retains the [xpcObject].
//   - https://developer.apple.com/documentation/xpc/xpc_retain?language=objc
func (x *xpcObject) retain() {
	C.xpcRetain(objc.Ptr(x))
}

// releaseOnCleanup registers a cleanup function to release the [xpcObject] when cleaned up.
//   - https://developer.apple.com/documentation/xpc/xpc_release?language=objc
func (x *xpcObject) releaseOnCleanup() {
	runtime.AddCleanup(x, func(p unsafe.Pointer) {
		C.xpcRelease(p)
	}, objc.Ptr(x))
}

// Retain calls retain method on the given object and returns it.
func Retain[T interface{ retain() }](o T) T {
	o.retain()
	return o
}

// ReleaseOnCleanup calls releaseOnCleanup method on the given object and returns it.
func ReleaseOnCleanup[T interface{ releaseOnCleanup() }](o T) T {
	o.releaseOnCleanup()
	return o
}
