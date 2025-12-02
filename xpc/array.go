package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"iter"
	"runtime/cgo"
	"unsafe"
)

// Array represents an XPC array([XPC_TYPE_ARRAY]) object. [TypeArray]
//
// [XPC_TYPE_ARRAY]: https://developer.apple.com/documentation/xpc/xpc_type_array-c.macro?language=objc
type Array struct {
	*XpcObject
}

var _ Object = &Array{}

// NewArray creates a new [Array] from the given [Object]s.
//
//   - https://developer.apple.com/documentation/xpc/xpc_array_create(_:_:)?language=objc
func NewArray(objects ...Object) *Array {
	cObjects := make([]unsafe.Pointer, len(objects))
	for i, obj := range objects {
		cObjects[i] = obj.Raw()
	}
	return ReleaseOnCleanup(&Array{XpcObject: &XpcObject{C.xpcArrayCreate(
		(*unsafe.Pointer)(unsafe.Pointer(&cObjects[0])),
		C.size_t(len(cObjects)),
	)}})
}

// Count returns the number of elements in the [Array].
//
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_count(_:)?language=objc
func (a *Array) Count() int {
	return int(C.xpcArrayGetCount(a.Raw()))
}

// ArrayApplier is a function type for applying to each element in the Array.
type ArrayApplier func(uint64, Object) bool

// callArrayApplier is called from C to apply a function to each element in the Array.
//
//export callArrayApplier
func callArrayApplier(cgoApplier uintptr, index C.size_t, cgoValue uintptr) C.bool {
	applier := unwrapHandler[ArrayApplier](cgoApplier)
	value := unwrapObject[Object](cgoValue)
	result := applier(uint64(index), value)
	return C.bool(result)
}

// All iterates over all elements in the [Array].
//
//   - https://developer.apple.com/documentation/xpc/xpc_array_apply(_:_:)?language=objc
func (a *Array) All() iter.Seq2[uint64, Object] {
	return func(yieald func(uint64, Object) bool) {
		cgoApplier := cgo.NewHandle(ArrayApplier(yieald))
		defer cgoApplier.Delete()
		_ = C.xpcArrayApply(
			a.Raw(),
			C.uintptr_t(cgoApplier),
		)
	}
}

// Values iterates over all values in the [Array] using [Array.All].
func (a *Array) Values() iter.Seq[Object] {
	return func(yieald func(Object) bool) {
		for _, value := range a.All() {
			if !yieald(value) {
				return
			}
		}
	}
}
