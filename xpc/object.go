package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// MARK: - Object: Untyped XPC Object

// Object represents an untyped XPC object ([xpc_object_t]).
//
// [xpc_object_t]: https://developer.apple.com/documentation/xpc/xpc_object_t?language=objc
type Object interface {
	objc.NSObject
	String() string    // String returns the description of the XPC object.
	releaseOnCleanup() // releaseOnCleanup releases the XPC object on cleanup.
}

// GetType returns the [Type] of the given [Object].
//   - https://developer.apple.com/documentation/xpc/xpc_get_type(_:)?language=objc
func GetType(o Object) Type {
	return Type{C.xpcGetType(objc.Ptr(o))}
}

// NewObject creates a new [Object] from an existing [xpc_object_t].
// The XPC APIs should be wrapped in C to use void* instead of [xpc_object_t].
// This function accepts an [unsafe.Pointer] that represents void* in C.
// It determines the specific type and returns the appropriate wrapper.
//
// [xpc_object_t]: https://developer.apple.com/documentation/xpc/xpc_object_t?language=objc
func NewObject(o unsafe.Pointer) Object {
	if o == nil {
		return nil
	}
	xpcObject := newXpcObject(o)
	// Determine the specific type and return the appropriate wrapper.
	// It allows users to use type assertions to access type-specific methods.
	switch GetType(xpcObject) {
	case TypeArray:
		return &Array{xpcObject}
	case TypeBool:
		return &Bool{xpcObject}
	case TypeData:
		return &Data{xpcObject}
	case TypeDate:
		return &Date{xpcObject}
	case TypeDictionary:
		return &Dictionary{xpcObject}
	case TypeDouble:
		return &Double{xpcObject}
	case TypeFD:
		return &Fd{xpcObject}
	case TypeInt64:
		return &Int64{xpcObject}
	case TypeNull:
		return &Null{xpcObject}
	case TypeRichError:
		return &RichError{xpcObject}
	case TypeSession:
		return &Session{xpcObject: xpcObject}
	case TypeString:
		return &String{xpcObject}
	case TypeUInt64:
		return &UInt64{xpcObject}
	case TypeUUID:
		return &UUID{xpcObject}
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

// MARK: - Bool: XPC_TYPE_BOOL represents an XPC boolean object.

// Bool represents an XPC boolean([XPC_TYPE_BOOL]) object. [TypeBool]
//
// [XPC_TYPE_BOOL]: https://developer.apple.com/documentation/xpc/xpc_type_bool-c.macro?language=objc
type Bool struct{ *xpcObject }

var _ Object = &Bool{}

// NewBool returns a new [Bool] object from the given Go bool.
//   - https://developer.apple.com/documentation/xpc/xpc_bool_create(_:)?language=objc
func NewBool(b bool) *Bool {
	cbool := C.bool(b)
	return ReleaseOnCleanup(&Bool{newXpcObject(C.xpcBoolCreate(cbool))})
}

// Value returns the boolean value of the [Bool] object.
//   - https://developer.apple.com/documentation/xpc/xpc_bool_get_value(_:)?language=objc
func (b *Bool) Bool() bool {
	return bool(C.xpcBoolGetValue(objc.Ptr(b)))
}

var (
	BoolTrue  = &Bool{newXpcObject(C.xpcBoolTrue())}
	BoolFalse = &Bool{newXpcObject(C.xpcBoolFalse())}
)

// MARK: - Data: XPC_TYPE_DATA represents an XPC data object.

// Data represents an XPC data([XPC_TYPE_DATA]) object. [TypeData]
//
// [XPC_TYPE_DATA]: https://developer.apple.com/documentation/xpc/xpc_type_data-c.macro?language=objc
type Data struct{ *xpcObject }

var _ Object = &Data{}

// NewData returns a new [Data] object from the given byte slice.
//   - https://developer.apple.com/documentation/xpc/xpc_data_create(_:_:)?language=objc
func NewData(b []byte) *Data {
	if len(b) == 0 {
		return ReleaseOnCleanup(&Data{newXpcObject(C.xpcDataCreate(nil, 0))})
	}
	return ReleaseOnCleanup(&Data{newXpcObject(C.xpcDataCreate(
		unsafe.Pointer(&b[0]),
		C.size_t(len(b)),
	))})
}

// Bytes returns the byte slice of the [Data] object.
//   - https://developer.apple.com/documentation/xpc/xpc_data_get_bytes_ptr(_:)?language=objc
//   - https://developer.apple.com/documentation/xpc/xpc_data_get_length(_:)?language=objc
func (d *Data) Bytes() []byte {
	size := C.xpcDataGetLength(objc.Ptr(d))
	ptr := C.xpcDataGetBytesPtr(objc.Ptr(d))
	if ptr == nil || size == 0 {
		return nil
	}
	return C.GoBytes(ptr, C.int(size))
}

// MARK: - Double: XPC_TYPE_DOUBLE represents an XPC double object.

// Double represents an XPC double([XPC_TYPE_DOUBLE]) object. [TypeDouble]
//
// [XPC_TYPE_DOUBLE]: https://developer.apple.com/documentation/xpc/xpc_type_double-c.macro?language=objc
type Double struct{ *xpcObject }

var _ Object = &Double{}

// NewDouble returns a new [Double] object from the given Go float64.
//   - https://developer.apple.com/documentation/xpc/xpc_double_create(_:)?language=objc
func NewDouble(f float64) *Double {
	cdouble := C.double(f)
	return ReleaseOnCleanup(&Double{newXpcObject(C.xpcDoubleCreate(cdouble))})
}

// Value returns the float64 value of the [Double] object.
//   - https://developer.apple.com/documentation/xpc/xpc_double_get_value(_:)?language=objc
func (d *Double) Float64() float64 {
	return float64(C.xpcDoubleGetValue(objc.Ptr(d)))
}

// MARK: - Int64: XPC_TYPE_INT64 represents an XPC int64 object.

// Int64 represents an XPC int64([XPC_TYPE_INT64]) object. [TypeInt64]
//
// [XPC_TYPE_INT64]: https://developer.apple.com/documentation/xpc/xpc_type_int64-c.macro?language=objc
type Int64 struct{ *xpcObject }

var _ Object = &Int64{}

// NewInt64 returns a new [Int64] object from the given Go int64.
//   - https://developer.apple.com/documentation/xpc/xpc_int64_create(_:)?language=objc
func NewInt64(i int64) *Int64 {
	cint64 := C.int64_t(i)
	return ReleaseOnCleanup(&Int64{newXpcObject(C.xpcInt64Create(cint64))})
}

// Value returns the int64 value of the [Int64] object.
//   - https://developer.apple.com/documentation/xpc/xpc_int64_get_value(_:)?language=objc
func (i *Int64) Int64() int64 {
	return int64(C.xpcInt64GetValue(objc.Ptr(i)))
}

// MARK: - UInt64: XPC_TYPE_UINT64 represents an XPC uint64 object.

// UInt64 represents an XPC uint64([XPC_TYPE_UINT64]) object. [TypeUInt64]
//
// [XPC_TYPE_UINT64]: https://developer.apple.com/documentation/xpc/xpc_type_uint64-c.macro?language=objc
type UInt64 struct{ *xpcObject }

var _ Object = &UInt64{}

// NewUInt64 returns a new [UInt64] object from the given Go uint64.
//   - https://developer.apple.com/documentation/xpc/xpc_uint64_create(_:)?language=objc
func NewUInt64(u uint64) *UInt64 {
	cuint64 := C.uint64_t(u)
	return ReleaseOnCleanup(&UInt64{newXpcObject(C.xpcUInt64Create(cuint64))})
}

// Value returns the uint64 value of the [UInt64] object.
//   - https://developer.apple.com/documentation/xpc/xpc_uint64_get_value(_:)?language=objc
func (u *UInt64) UInt64() uint64 {
	return uint64(C.xpcUInt64GetValue(objc.Ptr(u)))
}

// MARK: - String: XPC_TYPE_STRING represents an XPC string object.

// String represents an XPC string([XPC_TYPE_STRING]) object. [TypeString]
//
// [XPC_TYPE_STRING]: https://developer.apple.com/documentation/xpc/xpc_type_string-c.macro?language=objc
type String struct{ *xpcObject }

var _ Object = &String{}

// NewString returns a new [String] object from the given Go string.
//   - https://developer.apple.com/documentation/xpc/xpc_string_create(_:)?language=objc
func NewString(s string) *String {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	return ReleaseOnCleanup(&String{newXpcObject(C.xpcStringCreate(cstr))})
}

// String returns the Go string value of the [String] object.
//   - https://developer.apple.com/documentation/xpc/xpc_string_get_string_ptr(_:)?language=objc
func (s *String) String() string {
	cstr := C.xpcStringGetStringPtr(objc.Ptr(s))
	len := C.xpcStringGetLength(objc.Ptr(s))
	if cstr == nil || len == 0 {
		return ""
	}
	return C.GoStringN(cstr, C.int(len))
}

// MARK: - Fd: XPC_TYPE_FD represents an XPC file descriptor object.

// Fd represents an XPC file descriptor([XPC_TYPE_FD]) object. [TypeFd]
//
// [XPC_TYPE_FD]: https://developer.apple.com/documentation/xpc/xpc_type_fd-c.macro?language=objc
type Fd struct{ *xpcObject }

var _ Object = &Fd{}

// NewFd returns a new [Fd] object from the given file descriptor.
//   - https://developer.apple.com/documentation/xpc/xpc_fd_create(_:)?language=objc
func NewFd(fd uintptr) *Fd {
	cfd := C.int(fd)
	return ReleaseOnCleanup(&Fd{newXpcObject(C.xpcFdCreate(cfd))})
}

// Dup returns a duplicated file descriptor from the [Fd] object.
//   - https://developer.apple.com/documentation/xpc/xpc_fd_dup(_:)?language=objc
func (f *Fd) Dup() uintptr {
	return uintptr(C.xpcFdDup(objc.Ptr(f)))
}

// MARK: - Date: XPC_TYPE_DATE represents an XPC date object.

// Date represents an XPC date([XPC_TYPE_DATE]) object. [TypeDate]
//
// [XPC_TYPE_DATE]: https://developer.apple.com/documentation/xpc/xpc_type_date-c.macro?language=objc
type Date struct{ *xpcObject }

var _ Object = &Date{}

// NewDate returns a new [Date] object from the given Go int64 representing nanoseconds since epoch.
//   - https://developer.apple.com/documentation/xpc/xpc_date_create(_:)?language=objc
func NewDate(nanoseconds int64) *Date {
	cinterval := C.int64_t(nanoseconds)
	return ReleaseOnCleanup(&Date{newXpcObject(C.xpcDateCreate(cinterval))})
}

// NewDateFromCurrent returns a new [Date] object representing the current date and time.
//   - https://developer.apple.com/documentation/xpc/xpc_date_from_current()?language=objc
func NewDateFromCurrent() *Date {
	return ReleaseOnCleanup(&Date{newXpcObject(C.xpcDateCreateFromCurrent())})
}

// Value returns the [time.Time] value of the [Date] object.
//   - https://developer.apple.com/documentation/xpc/xpc_date_get_value(_:)?language=objc
func (d *Date) Time() time.Time {
	unixNano := int64(C.xpcDateGetValue(objc.Ptr(d)))
	return time.Unix(0, unixNano)
}

// MARK: - UUID: XPC_TYPE_UUID represents an XPC UUID object.

// UUID represents an XPC UUID([XPC_TYPE_UUID]) object. [TypeUUID]
//
// [XPC_TYPE_UUID]: https://developer.apple.com/documentation/xpc/xpc_type_uuid-c.macro?language=objc
type UUID struct{ *xpcObject }

var _ Object = &UUID{}

// NewUUID returns a new [UUID] object from the given UUID byte array.
//   - https://developer.apple.com/documentation/xpc/xpc_uuid_create(_:)?language=objc
func NewUUID(uuid [16]byte) *UUID {
	cuuid := (*C.uint8_t)(unsafe.Pointer(&uuid[0]))
	return ReleaseOnCleanup(&UUID{newXpcObject(C.xpcUUIDCreate(cuuid))})
}

// Bytes returns the UUID byte array of the [UUID] object.
//   - https://developer.apple.com/documentation/xpc/xpc_uuid_get_bytes(_:)?language=objc
func (u *UUID) Bytes() [16]byte {
	var uuid [16]byte
	ptr := C.xpcUUIDGetBytes(objc.Ptr(u))
	if ptr == nil {
		return uuid
	}
	copy(uuid[:], unsafe.Slice((*byte)(ptr), 16))
	return uuid
}

// MARK: - Shared Memory: XPC_TYPE_SHMEM represents an XPC shared memory object.
// MARK: - Null: XPC_TYPE_NULL represents an XPC null object.

// Null represents an XPC null([XPC_TYPE_NULL]) object. [TypeNull]
//
// [XPC_TYPE_NULL]: https://developer.apple.com/documentation/xpc/xpc_type_null-c.macro?language=objc
type Null struct{ *xpcObject }

var _ Object = &Null{}

// NewNull returns a new [Null] object.
//   - https://developer.apple.com/documentation/xpc/xpc_null_create()?language=objc
func NewNull() *Null {
	return ReleaseOnCleanup(&Null{newXpcObject(C.xpcNullCreate())})
}
