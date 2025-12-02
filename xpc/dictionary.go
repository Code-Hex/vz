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
	"unsafe"
)

// Dictionary represents an XPC dictionary (XPC_TYPE_DICTIONARY) object. [TypeDictionary]
//   - https://developer.apple.com/documentation/xpc/xpc_type_dictionary-c.macro?language=objc
type Dictionary struct {
	*XpcObject
}

var _ Object = &Dictionary{}

// NewDictionary creates a new empty [Dictionary] object and applies the given entries.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_create_empty()?language=objc
//
// The entries can be created using [DictionaryEntry] functions such as [KeyValue].
func NewDictionary(entries ...DictionaryEntry) *Dictionary {
	d := ReleaseOnCleanup(&Dictionary{XpcObject: &XpcObject{C.xpcDictionaryCreateEmpty()}})
	for _, e := range entries {
		e(d)
	}
	return d
}

// DictionaryEntry defines a function type for customizing [NewDictionary] or [Dictionary.CreateReply].
type DictionaryEntry func(*Dictionary)

// KeyValue sets a [Object] value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_value(_:_:_:)?language=objc
func KeyValue(key string, val Object) DictionaryEntry {
	return func(o *Dictionary) {
		o.SetValue(key, val)
	}
}

// DictionaryApplier is a function type for applying to each key-value pair in the XPC dictionary.
type DictionaryApplier func(string, Object) bool

// callDictionaryApplier is called from C to apply a function to each key-value pair in the XPC dictionary object.
//
//export callDictionaryApplier
func callDictionaryApplier(cgoApplier uintptr, cKey *C.char, cgoValue uintptr) C.bool {
	applier := unwrapHandler[DictionaryApplier](cgoApplier)
	return C.bool(applier(C.GoString(cKey), unwrapObject[Object](cgoValue)))
}

// All iterates over all key-value pairs in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_apply(_:_:)?language=objc
func (o *Dictionary) All() iter.Seq2[string, Object] {
	return func(yieald func(string, Object) bool) {
		cgoApplier := cgo.NewHandle(DictionaryApplier(yieald))
		defer cgoApplier.Delete()
		C.xpcDictionaryApply(o.Raw(), C.uintptr_t(cgoApplier))
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

// GetData retrieves a byte slice value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_data(_:_:_:)?language=objc
//
// Returns nil if the key does not exist.
func (o *Dictionary) GetData(key string) []byte {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	var n C.size_t
	p := C.xpcDictionaryGetData(o.Raw(), cKey, &n)
	if p == nil || n == 0 {
		return nil
	}
	return C.GoBytes(p, C.int(n))
}

// GetString retrieves a string value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_string(_:_:)?language=objc
//
// Returns an empty string if the key does not exist.
func (o *Dictionary) GetString(key string) string {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	val := C.xpcDictionaryGetString(o.Raw(), cKey)
	return C.GoString(val)
}

// SetValue sets an [Object] value for the given key in the [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_set_value(_:_:_:)?language=objc
func (o *Dictionary) SetValue(key string, val Object) {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	C.xpcDictionarySetValue(o.Raw(), cKey, val.Raw())
}

// GetValue retrieves an [Object] value from the [Dictionary] by key.
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_get_value(_:_:)?language=objc
//
// Returns nil if the key does not exist.
func (o *Dictionary) GetValue(key string) Object {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	val := C.xpcDictionaryGetValue(o.Raw(), cKey)
	if val == nil {
		return nil
	}
	return NewObject(val)
}

// DictionaryCreateReply creates a new reply [Dictionary] based on the current [Dictionary].
//   - https://developer.apple.com/documentation/xpc/xpc_dictionary_create_reply(_:)?language=objc
//
// The entries can be created using [DictionaryEntry] functions such as [KeyValue].
func (o *Dictionary) CreateReply(entries ...DictionaryEntry) *Dictionary {
	// Do not use ReleaseOnCleanup here because the reply dictionary will be released in C after sending.
	d := &Dictionary{XpcObject: &XpcObject{C.xpcDictionaryCreateReply(o.Raw())}}
	for _, entry := range entries {
		entry(d)
	}
	return d
}

// SenderSatisfies checks if the sender of the message [Dictionary] satisfies the given [PeerRequirement].
//   - https://developer.apple.com/documentation/xpc/xpc_peer_requirement_match_received_message?language=objc
func (d *Dictionary) SenderSatisfies(requirement *PeerRequirement) (bool, error) {
	var err_out unsafe.Pointer
	res := C.xpcPeerRequirementMatchReceivedMessage(requirement.Raw(), d.Raw(), &err_out)
	if err_out != nil {
		return false, fmt.Errorf("error matching peer requirement: %w", newRichError(err_out))
	}
	return bool(res), nil
}
