package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"fmt"
	"iter"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/cgohandler"
	"github.com/Code-Hex/vz/v3/internal/objc"
)

// Dictionary represents an XPC dictionary ([XPC_TYPE_DICTIONARY]) object. [TypeDictionary]
//
// [XPC_TYPE_DICTIONARY]: https://developer.apple.com/documentation/xpc/xpc_type_dictionary-c.macro?language=objc
type Dictionary struct {
	*xpcObject
}

var _ Object = &Dictionary{}

// MARK: - Constructor

// NewDictionary creates a new empty [Dictionary] object and applies the given entries.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_create_empty()?language=objc
//
// The entries can be created using [DictionaryEntry] functions such as [KeyValue].
func NewDictionary(entries ...DictionaryEntry) *Dictionary {
	d := ReleaseOnCleanup(&Dictionary{newXpcObject(C.xpcDictionaryCreateEmpty())})
	for _, e := range entries {
		e(d)
	}
	return d
}

// DictionaryEntry defines a function type for customizing [NewDictionary] or [Dictionary.CreateReply].
type DictionaryEntry func(*Dictionary)

// KeyValue sets a [Object] value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_value(_:_:_:)?language=objc
func KeyValue(key string, value Object) DictionaryEntry {
	return func(o *Dictionary) {
		o.SetValue(key, value)
	}
}

// MARK: - CreateReply

// DictionaryCreateReply creates a new reply [Dictionary] based on the current [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_create_reply(_:)?language=objc
//
// The entries can be created using [DictionaryEntry] functions such as [KeyValue].
func (o *Dictionary) CreateReply(entries ...DictionaryEntry) *Dictionary {
	// Do not use ReleaseOnCleanup here because the reply dictionary will be released in C after sending.
	d := &Dictionary{newXpcObject(C.xpcDictionaryCreateReply(objc.Ptr(o)))}
	for _, entry := range entries {
		entry(d)
	}
	return d
}

// MARK: - Value Accessors

// SetValue sets an [Object] value for the given key in the [Dictionary].
// The value may be nil, which removes the key from the dictionary.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_value(_:_:_:)?language=objc
func (o *Dictionary) SetValue(key string, value Object) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetValue(objc.Ptr(o), cKey, objc.Ptr(value))
}

// Count returns the number of key-value pairs in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_count(_:)?language=objc
func (o *Dictionary) Count() int {
	return int(C.xpcDictionaryGetCount(objc.Ptr(o)))
}

// GetValue retrieves an [Object] value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_value(_:_:)?language=objc
//
// Returns nil if the key does not exist.
func (o *Dictionary) GetValue(key string) Object {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	value := C.xpcDictionaryGetValue(objc.Ptr(o), cKey)
	if value == nil {
		return nil
	}
	return NewObject(value)
}

// MARK: - Iteration

// DictionaryApplier is a function type for applying to each key-value pair in the XPC dictionary.
type DictionaryApplier func(string, Object) bool

// callDictionaryApplier is called from C to apply a function to each key-value pair in the XPC dictionary object.
//
//export callDictionaryApplier
func callDictionaryApplier(cgoApplier uintptr, cKey *C.char, cgoValue uintptr) C.bool {
	applier := cgohandler.Unwrap[DictionaryApplier](cgoApplier)
	return C.bool(applier(C.GoString(cKey), unwrapObject[Object](cgoValue)))
}

// All iterates over all key-value pairs in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_apply(_:_:)?language=objc
func (o *Dictionary) All() iter.Seq2[string, Object] {
	return func(yieald func(string, Object) bool) {
		cgoApplier := cgo.NewHandle(DictionaryApplier(yieald))
		defer cgoApplier.Delete()
		C.xpcDictionaryApply(objc.Ptr(o), C.uintptr_t(cgoApplier))
	}
}

// Keys iterates over all keys in the [Dictionary] using [Dictionary.All].
func (o *Dictionary) Keys() iter.Seq[string] {
	return func(yieald func(string) bool) {
		for key := range o.All() {
			if !yieald(key) {
				return
			}
		}
	}
}

// Values iterates over all [Object] values in the [Dictionary] using [Dictionary.All].
func (o *Dictionary) Values() iter.Seq[Object] {
	return func(yieald func(Object) bool) {
		for _, value := range o.All() {
			if !yieald(value) {
				return
			}
		}
	}
}

// Entries iterates over all [DictionaryEntry] entries in the [Dictionary] using [Dictionary.All].
func (o *Dictionary) Entries() iter.Seq[DictionaryEntry] {
	return func(yieald func(DictionaryEntry) bool) {
		for key, value := range o.All() {
			if !yieald(KeyValue(key, value)) {
				return
			}
		}
	}
}

// MARK: - Typed Getters

// DupFd retrieves a duplicated file descriptor from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_dup_fd(_:_:)?language=objc
func (o *Dictionary) DupFd(key string) uintptr {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	return uintptr(C.xpcDictionaryDupFd(objc.Ptr(o), cKey))
}

// GetArray retrieves an [Array] value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_array(_:_:)?language=objc
//
// Returns nil if the key does not exist.
func (o *Dictionary) GetArray(key string) *Array {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	p := C.xpcDictionaryGetArray(objc.Ptr(o), cKey)
	if p == nil {
		return nil
	}
	return &Array{newXpcObject(p)}
}

// GetBool retrieves a boolean value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_bool(_:_:)?language=objc
func (o *Dictionary) GetBool(key string) bool {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	value := C.xpcDictionaryGetBool(objc.Ptr(o), cKey)
	return bool(value)
}

// GetData retrieves a byte slice value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_data(_:_:_:)?language=objc
//
// Returns nil if the key does not exist.
func (o *Dictionary) GetData(key string) []byte {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	var n C.size_t
	p := C.xpcDictionaryGetData(objc.Ptr(o), cKey, &n)
	if p == nil || n == 0 {
		return nil
	}
	return C.GoBytes(p, C.int(n))
}

// GetDate retrieves a date value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_date(_:_:)?language=objc
func (o *Dictionary) GetDate(key string) time.Time {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	unixNano := C.xpcDictionaryGetDate(objc.Ptr(o), cKey)
	return time.Unix(0, int64(unixNano))
}

// GetDictionary retrieves a [Dictionary] value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_dictionary(_:_:)?language=objc
func (o *Dictionary) GetDictionary(key string) *Dictionary {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	p := C.xpcDictionaryGetDictionary(objc.Ptr(o), cKey)
	if p == nil {
		return nil
	}
	return &Dictionary{newXpcObject(p)}
}

// GetDouble retrieves a double value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_double(_:_:)?language=objc
func (o *Dictionary) GetDouble(key string) float64 {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	value := C.xpcDictionaryGetDouble(objc.Ptr(o), cKey)
	return float64(value)
}

// GetInt64 retrieves an int64 value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_int64(_:_:)?language=objc
func (o *Dictionary) GetInt64(key string) int64 {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	value := C.xpcDictionaryGetInt64(objc.Ptr(o), cKey)
	return int64(value)
}

// GetString retrieves a string value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_string(_:_:)?language=objc
//
// Returns an empty string if the key does not exist.
func (o *Dictionary) GetString(key string) string {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	value := C.xpcDictionaryGetString(objc.Ptr(o), cKey)
	return C.GoString(value)
}

// GetUInt64 retrieves a uint64 value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_uint64(_:_:)?language=objc
func (o *Dictionary) GetUInt64(key string) uint64 {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	value := C.xpcDictionaryGetUInt64(objc.Ptr(o), cKey)
	return uint64(value)
}

// GetUUID retrieves a UUID value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_uuid(_:_:)?language=objc
func (o *Dictionary) GetUUID(key string) [16]byte {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	var uuid [16]byte
	ptr := C.xpcDictionaryGetUUID(objc.Ptr(o), cKey)
	if ptr == nil {
		return uuid
	}
	copy(uuid[:], unsafe.Slice((*byte)(ptr), 16))
	return uuid
}

// MARK: - Typed Setters

// SetBool sets a boolean value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_bool(_:_:_:)?language=objc
func (o *Dictionary) SetBool(key string, value bool) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetBool(objc.Ptr(o), cKey, C.bool(value))
}

// SetData sets a byte slice value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_data(_:_:_:_:)?language=objc
func (o *Dictionary) SetData(key string, data []byte) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	var ptr unsafe.Pointer
	var length C.size_t
	if len(data) > 0 {
		ptr = unsafe.Pointer(&data[0])
		length = C.size_t(len(data))
	}
	C.xpcDictionarySetData(objc.Ptr(o), cKey, ptr, length)
}

// SetDate sets a date value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_date(_:_:_:)?language=objc
func (o *Dictionary) SetDate(key string, t time.Time) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	timestamp := C.int64_t(t.UnixNano())
	C.xpcDictionarySetDate(objc.Ptr(o), cKey, timestamp)
}

// SetDouble sets a double value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_double(_:_:_:)?language=objc
func (o *Dictionary) SetDouble(key string, value float64) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetDouble(objc.Ptr(o), cKey, C.double(value))
}

// SetFd sets a file descriptor value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_fd(_:_:_:)?language=objc
func (o *Dictionary) SetFd(key string, fd uintptr) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetFd(objc.Ptr(o), cKey, C.int(fd))
}

// SetInt64 sets an int64 value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_int64(_:_:_:)?language=objc
func (o *Dictionary) SetInt64(key string, value int64) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetInt64(objc.Ptr(o), cKey, C.int64_t(value))
}

// SetString sets a string value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_string(_:_:_:)?language=objc
func (o *Dictionary) SetString(key string, value string) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))
	C.xpcDictionarySetString(objc.Ptr(o), cKey, cValue)
}

// SetUInt64 sets a uint64 value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_uint64(_:_:_:)?language=objc
func (o *Dictionary) SetUInt64(key string, value uint64) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetUInt64(objc.Ptr(o), cKey, C.uint64_t(value))
}

// SetUUID sets a UUID value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_uuid(_:_:_:)?language=objc
func (o *Dictionary) SetUUID(key string, uuid [16]byte) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetUUID(objc.Ptr(o), cKey, (*C.uint8_t)(unsafe.Pointer(&uuid[0])))
}

// MARK: - Peer Requirement

// SenderSatisfies checks if the sender of the message [Dictionary] satisfies the given [PeerRequirement].
//   - https://developer.apple.com/documentation/xpc/xpc_peer_requirement_match_received_message?language=objc
func (d *Dictionary) SenderSatisfies(requirement *PeerRequirement) (bool, error) {
	var err_out unsafe.Pointer
	res := C.xpcPeerRequirementMatchReceivedMessage(objc.Ptr(requirement), objc.Ptr(d), &err_out)
	if err_out != nil {
		return false, fmt.Errorf("error matching peer requirement: %w", newRichError(err_out))
	}
	return bool(res), nil
}
