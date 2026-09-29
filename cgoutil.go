package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

// pointer is a type alias which is able to use as embedded type and
// makes as unexported it.
type pointer = objc.Pointer

// NSError indicates NSError.
type NSError struct {
	Domain               string
	Code                 int
	LocalizedDescription string
	UserInfo             string
}

func newNSErrorAsNil() unsafe.Pointer { return nil }

func (n *NSError) Error() string {
	if n == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"Error Domain=%s Code=%d Description=%q UserInfo=%s",
		n.Domain,
		n.Code,
		n.LocalizedDescription,
		n.UserInfo,
	)
}

func newNSError(p unsafe.Pointer) *NSError {
	if p == nil {
		return nil
	}
	object := objc.NewManagedPointer(p, vzbridge.ReleaseObject)
	defer objc.Release(object)
	return &NSError{
		Domain:               nativeString(vzbridge.ErrorDomain(object)),
		Code:                 int(vzbridge.ErrorCode(object)),
		LocalizedDescription: nativeString(vzbridge.ErrorDescription(object)),
		UserInfo:             nativeString(vzbridge.ErrorUserInfo(object)),
	}
}

func nativeString(object *objc.Pointer) string {
	if object == nil {
		return ""
	}
	defer objc.Release(object)
	return string(nativeBytes(vzbridge.ObjectStringData(object)))
}

func nativeBytes(object *objc.Pointer) []byte {
	if object == nil {
		return nil
	}
	defer objc.Release(object)
	size := vzbridge.DataLength(object)
	if size == 0 {
		return nil
	}
	data := vzbridge.DataBytes(object)
	return append([]byte(nil), unsafe.Slice((*byte)(data), int(size))...)
}

func nativeArray(object *objc.Pointer) []*objc.Pointer {
	if object == nil {
		return nil
	}
	defer objc.Release(object)
	objects := make([]*objc.Pointer, int(vzbridge.ArrayCount(object)))
	for i := range objects {
		objects[i] = vzbridge.ArrayObject(object, uint64(i))
	}
	return objects
}

func nativeObjectArray[T objc.NSObject](objects []T) *objc.Pointer {
	array := vzbridge.NewArray()
	for _, object := range objects {
		vzbridge.ArrayAppend(array, object)
	}
	return array
}

func nativeObjectDictionary[T objc.NSObject](objects map[string]T) *objc.Pointer {
	dictionary := vzbridge.NewDictionary()
	for key, object := range objects {
		vzbridge.DictionarySet(dictionary, key, object)
	}
	return dictionary
}
