package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
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
	info := vzbridge.NSError_UserInfo(object)
	defer objc.Release(info)
	return &NSError{
		Domain:               nativeString(vzbridge.NSError_Domain(object)),
		Code:                 int(vzbridge.NSError_Code(object)),
		LocalizedDescription: nativeString(vzbridge.NSError_LocalizedDescription(object)),
		UserInfo:             nativeString(vzbridge.NSObject_Description(info)),
	}
}

func nativeString(object *objc.Pointer) string {
	if object == nil {
		return ""
	}
	defer objc.Release(object)
	return string(nativeBytes(vzbridge.NSString_DataUsingEncoding(object, vzbridge.NSUTF8StringEncoding)))
}

func nativeBytes(object *objc.Pointer) []byte {
	if object == nil {
		return nil
	}
	defer objc.Release(object)
	size := vzbridge.NSData_Length(object)
	if size == 0 {
		return nil
	}
	data := vzbridge.NSData_Bytes(object)
	return append([]byte(nil), unsafe.Slice((*byte)(data), int(size))...)
}

func nativeArray(object *objc.Pointer) []*objc.Pointer {
	if object == nil {
		return nil
	}
	defer objc.Release(object)
	objects := make([]*objc.Pointer, int(vzbridge.NSArray_Count(object)))
	for i := range objects {
		objects[i] = vzbridge.NSArray_ObjectAtIndex(object, uint64(i))
	}
	return objects
}

func nativeObjectArray[T objc.NSObject](objects []T) *objc.Pointer {
	array := vzbridge.NSMutableArray_New()
	for _, object := range objects {
		vzbridge.NSMutableArray_AddObject(array, object)
	}
	return array
}

func nativeObjectDictionary[T objc.NSObject](objects map[string]T) *objc.Pointer {
	dictionary := vzbridge.NSMutableDictionary_New()
	for key, object := range objects {
		vzbridge.NSMutableDictionary_SetObject_ForKey(dictionary, object, vzbridge.NSString_StringWithUTF8String(key))
	}
	return dictionary
}
