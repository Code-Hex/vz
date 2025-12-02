package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"runtime"
	"unsafe"
)

// XpcObject wraps an XPC object (xpc_object_t).
// It is expected to be embedded in other structs as pointer to provide common functionality.
//   - https://developer.apple.com/documentation/xpc/xpc_object_t?language=objc
type XpcObject struct {
	p unsafe.Pointer
}

var _ Object = (*XpcObject)(nil)

// Raw returns the raw xpc_object_t as [unsafe.Pointer].
func (x *XpcObject) Raw() unsafe.Pointer {
	return x.p
}

// Type returns the [Type] of the [XpcObject].
//   - https://developer.apple.com/documentation/xpc/xpc_get_type(_:)?language=objc
func (x *XpcObject) Type() Type {
	return Type{C.xpcGetType(x.p)}
}

// String returns the description of the [XpcObject].
//   - https://developer.apple.com/documentation/xpc/xpc_copy_description(_:)?language=objc
func (x *XpcObject) String() string {
	cs := C.xpcCopyDescription(x.Raw())
	defer C.free(unsafe.Pointer(cs))
	return C.GoString(cs)
}

// retain retains the [XpcObject].
// It also uses [runtime.SetFinalizer] to call [XpcObject.release] when it is garbage collected.
//   - https://developer.apple.com/documentation/xpc/xpc_retain?language=objc
func (x *XpcObject) retain() {
	C.xpcRetain(x.p)
	_ = ReleaseOnCleanup(x)
}

// releaseOnCleanup registers a cleanup function to release the XpcObject when cleaned up.
//   - https://developer.apple.com/documentation/xpc/xpc_release?language=objc
func (x *XpcObject) releaseOnCleanup() {
	runtime.AddCleanup(x, func(p unsafe.Pointer) {
		C.xpcRelease(p)
	}, x.p)
}

// Retain calls retain method on the given object and returns it.
func Retain[T interface{ retain() }](o T) T {
	o.retain()
	return o
}

// ReleaseOnCleanup calls releaseOnCleanup method on the given object and returns it.
func ReleaseOnCleanup[O interface{ releaseOnCleanup() }](o O) O {
	o.releaseOnCleanup()
	return o
}
