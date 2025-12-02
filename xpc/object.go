package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"runtime/cgo"
	"unsafe"
)

// Object represents a generic XPC object.
//   - https://developer.apple.com/documentation/xpc/xpc_object_t?language=objc
type Object interface {
	Raw() unsafe.Pointer // Raw returns the raw xpc_object_t pointer.
	Type() Type          // Type returns the type of the XPC object.
	String() string      // String returns the description of the XPC object.
	retain()             // retain retains the XPC object.
	releaseOnCleanup()   // releaseOnCleanup releases the XPC object on cleanup.
}

// NewObject creates a new [Object] from an existing xpc_object_t.
// The XPC APIs should be wrapped in C to use void* instead of xpc_object_t.
// This function accepts an [unsafe.Pointer] that represents void* in C.
// It determines the specific type and returns the appropriate wrapper.
func NewObject(o unsafe.Pointer) Object {
	if o == nil {
		return nil
	}
	xpcObject := &XpcObject{o}
	// Determine the specific type and return the appropriate wrapper.
	// It allows users to use type assertions to access type-specific methods.
	switch xpcObject.Type() {
	case TypeArray:
		return &Array{xpcObject}
	case TypeData:
		return &Data{xpcObject}
	case TypeDictionary:
		return &Dictionary{xpcObject}
	case TypeRichError:
		return &RichError{xpcObject}
	case TypeSession:
		return &Session{XpcObject: xpcObject}
	case TypeString:
		return &String{xpcObject}
	default:
		return xpcObject
	}
}

// wrapRawObject wraps an existing xpc_object_t into an Object and returns a handle.
// intended to be called from C.
//
//export wrapRawObject
func wrapRawObject(ptr unsafe.Pointer) uintptr {
	o := NewObject(ptr)
	if o == nil {
		return 0
	}
	return uintptr(cgo.NewHandle(o))
}

// unwrapObject unwraps the [cgo.Handle] from the given uintptr and returns the associated Object.
// It also deletes the handle to avoid memory leaks.
func unwrapObject[T any](handle uintptr) T {
	if handle == 0 {
		var zero T
		return zero
	}
	defer cgo.Handle(handle).Delete()
	return cgo.Handle(handle).Value().(T)
}

// XPC_TYPE_DATA represents an XPC data object.

// NewData returns a new [Data] object from the given byte slice.
//   - https://developer.apple.com/documentation/xpc/xpc_data_create(_:_:)?language=objc
func NewData(b []byte) Object {
	if len(b) == 0 {
		return ReleaseOnCleanup(&Data{&XpcObject{C.xpcDataCreate(nil, 0)}})
	}
	return ReleaseOnCleanup(&Data{&XpcObject{C.xpcDataCreate(
		unsafe.Pointer(&b[0]),
		C.size_t(len(b)),
	)}})
}

// Data represents an XPC data([XPC_TYPE_DATA]) object. [TypeData]
//
// [XPC_TYPE_DATA]: https://developer.apple.com/documentation/xpc/xpc_type_data-c.macro?language=objc
type Data struct{ *XpcObject }

var _ Object = &Data{}

// NewString returns a new [String] object from the given Go string.
//   - https://developer.apple.com/documentation/xpc/xpc_string_create(_:)?language=objc
func NewString(s string) Object {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	return ReleaseOnCleanup(&String{&XpcObject{C.xpcStringCreate(cstr)}})
}

// String represents an XPC string([XPC_TYPE_STRING]) object. [TypeString]
//
// [XPC_TYPE_STRING]: https://developer.apple.com/documentation/xpc/xpc_type_string-c.macro?language=objc
type String struct{ *XpcObject }

var _ Object = &String{}
