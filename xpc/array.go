package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"iter"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// Array represents an XPC array([XPC_TYPE_ARRAY]) object. [TypeArray]
//
// [XPC_TYPE_ARRAY]: https://developer.apple.com/documentation/xpc/xpc_type_array-c.macro?language=objc
type Array struct {
	*xpcObject
}

var _ Object = &Array{}

// MARK: - Constructor

// NewArray creates a new [Array] from the given [Object]s.
//   - https://developer.apple.com/documentation/xpc/xpc_array_create(_:_:)?language=objc
func NewArray(objects ...Object) *Array {
	n := len(objects)
	if n == 0 {
		return ReleaseOnCleanup(&Array{newXpcObject(C.xpcArrayCreate(nil, 0))})
	}
	cObjects := make([]unsafe.Pointer, n)
	for i, obj := range objects {
		cObjects[i] = objc.Ptr(obj)
	}
	return ReleaseOnCleanup(&Array{newXpcObject(C.xpcArrayCreate(
		(*unsafe.Pointer)(unsafe.Pointer(&cObjects[0])),
		C.size_t(len(cObjects)),
	))})
}

// MARK: - Value Accessors

// GetValue retrieves the [Object] at the given index in the [Array].
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_value(_:_:)?language=objc
func (a *Array) GetValue(index int) Object {
	ptr := C.xpcArrayGetValue(objc.Ptr(a), C.size_t(index))
	return NewObject(ptr)
}

// SetValue sets the [Object] at the given index in the [Array].
// The value can not be nil.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_value(_:_:_:)?language=objc
func (a *Array) SetValue(index int, value Object) {
	C.xpcArraySetValue(objc.Ptr(a), C.size_t(index), objc.Ptr(value))
}

// AppendValue appends the given [Object] to the end of the [Array].
//   - https://developer.apple.com/documentation/xpc/xpc_array_append_value(_:_:)?language=objc
func (a *Array) AppendValue(value Object) {
	C.xpcArrayAppendValue(objc.Ptr(a), objc.Ptr(value))
}

// MARK: - Iteration

// Count returns the number of elements in the [Array].
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_count(_:)?language=objc
func (a *Array) Count() int {
	return int(C.xpcArrayGetCount(objc.Ptr(a)))
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
//   - https://developer.apple.com/documentation/xpc/xpc_array_apply(_:_:)?language=objc
func (a *Array) All() iter.Seq2[uint64, Object] {
	return func(yieald func(uint64, Object) bool) {
		cgoApplier := cgo.NewHandle(ArrayApplier(yieald))
		defer cgoApplier.Delete()
		_ = C.xpcArrayApply(
			objc.Ptr(a),
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

// MARK: - Typed Getters

// DupFd retrieves duplicated file descriptors from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_dup_fd(_:_:)?language=objc
func (a *Array) DupFd(index int) uintptr {
	return uintptr(C.xpcArrayDupFd(objc.Ptr(a), C.size_t(index)))
}

// GetArray retrieves an [Array] value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_array(_:_:)?language=objc
func (a *Array) GetArray(index int) *Array {
	ptr := C.xpcArrayGetArray(objc.Ptr(a), C.size_t(index))
	return &Array{newXpcObject(ptr)}
}

// GetBool retrieves a boolean value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_bool(_:_:)?language=objc
func (a *Array) GetBool(index int) bool {
	return bool(C.xpcArrayGetBool(objc.Ptr(a), C.size_t(index)))
}

// GetData retrieves a byte slice from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_data(_:_:_:)?language=objc
func (a *Array) GetData(index int) []byte {
	var length C.size_t
	dataPtr := C.xpcArrayGetData(objc.Ptr(a), C.size_t(index), &length)
	if dataPtr == nil || length == 0 {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(dataPtr), C.int(length))
}

// GetDate retrieves a date value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_date(_:_:)?language=objc
func (a *Array) GetDate(index int) time.Time {
	unixNano := C.xpcArrayGetDate(objc.Ptr(a), C.size_t(index))
	return time.Unix(0, int64(unixNano))
}

// GetDictionary retrieves a [Dictionary] value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_dictionary(_:_:)?language=objc
func (a *Array) GetDictionary(index int) *Dictionary {
	ptr := C.xpcArrayGetDictionary(objc.Ptr(a), C.size_t(index))
	return &Dictionary{newXpcObject(ptr)}
}

// GetDouble retrieves a double value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_double(_:_:)?language=objc
func (a *Array) GetDouble(index int) float64 {
	return float64(C.xpcArrayGetDouble(objc.Ptr(a), C.size_t(index)))
}

// GetInt64 retrieves an int64 value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_int64(_:_:)?language=objc
func (a *Array) GetInt64(index int) int64 {
	return int64(C.xpcArrayGetInt64(objc.Ptr(a), C.size_t(index)))
}

// GetString retrieves a string value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_string(_:_:)?language=objc
func (a *Array) GetString(index int) string {
	cstr := C.xpcArrayGetString(objc.Ptr(a), C.size_t(index))
	return C.GoString(cstr)
}

// GetUInt64 retrieves a uint64 value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_uint64(_:_:)?language=objc
func (a *Array) GetUInt64(index int) uint64 {
	return uint64(C.xpcArrayGetUInt64(objc.Ptr(a), C.size_t(index)))
}

// GetUUID retrieves a UUID value from the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_get_uuid(_:_:)?language=objc
func (a *Array) GetUUID(index int) [16]byte {
	var uuid [16]byte
	ptr := C.xpcArrayGetUUID(objc.Ptr(a), C.size_t(index))
	if ptr == nil {
		return uuid
	}
	copy(uuid[:], unsafe.Slice((*byte)(ptr), 16))
	return uuid
}

// MARK: - Typed Setters

// SetBool sets a boolean value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_bool(_:_:_:)?language=objc
func (a *Array) SetBool(index int, value bool) {
	cvalue := C.bool(value)
	C.xpcArraySetBool(objc.Ptr(a), C.size_t(index), cvalue)
}

// SetData sets a byte slice in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_data(_:_:_:_:)?language=objc
func (a *Array) SetData(index int, value []byte) {
	var ptr unsafe.Pointer
	var length C.size_t
	if len(value) > 0 {
		ptr = unsafe.Pointer(&value[0])
		length = C.size_t(len(value))
	}
	C.xpcArraySetData(objc.Ptr(a), C.size_t(index), ptr, length)
}

// SetDate sets a date value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_date(_:_:_:)?language=objc
func (a *Array) SetDate(index int, value time.Time) {
	unixNano := C.int64_t(value.UnixNano())
	C.xpcArraySetDate(objc.Ptr(a), C.size_t(index), unixNano)
}

// SetDouble sets a double value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_double(_:_:_:)?language=objc
func (a *Array) SetDouble(index int, value float64) {
	cvalue := C.double(value)
	C.xpcArraySetDouble(objc.Ptr(a), C.size_t(index), cvalue)
}

// SetFd sets a file descriptor in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_fd(_:_:_:)?language=objc
func (a *Array) SetFd(index int, fd uintptr) {
	cfd := C.int(fd)
	C.xpcArraySetFd(objc.Ptr(a), C.size_t(index), cfd)
}

// SetInt64 sets an int64 value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_int64(_:_:_:)?language=objc
func (a *Array) SetInt64(index int, value int64) {
	cvalue := C.int64_t(value)
	C.xpcArraySetInt64(objc.Ptr(a), C.size_t(index), cvalue)
}

// SetString sets a string value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_string(_:_:_:)?language=objc
func (a *Array) SetString(index int, value string) {
	cstr := C.CString(value)
	defer C.free(unsafe.Pointer(cstr))
	C.xpcArraySetString(objc.Ptr(a), C.size_t(index), cstr)
}

// SetUInt64 sets a uint64 value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_uint64(_:_:_:)?language=objc
func (a *Array) SetUInt64(index int, value uint64) {
	cvalue := C.uint64_t(value)
	C.xpcArraySetUInt64(objc.Ptr(a), C.size_t(index), cvalue)
}

// SetUUID sets a UUID value in the [Array] at the given index.
//   - https://developer.apple.com/documentation/xpc/xpc_array_set_uuid(_:_:_:)?language=objc
func (a *Array) SetUUID(index int, value [16]byte) {
	C.xpcArraySetUUID(objc.Ptr(a), C.size_t(index), (*C.uint8_t)(unsafe.Pointer(&value[0])))
}

// ArrayApppendIndex is a constant to append an element to the end of the [Array].
const ArrayApppendIndex = C.XPC_ARRAY_APPEND
