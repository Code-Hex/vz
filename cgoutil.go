package vz

import (
	"strings"
	"unicode/utf8"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// pointer is a type alias which is able to use as embedded type and
// makes as unexported it.
type pointer = objc.Pointer

// NSError indicates NSError.
type NSError = objcutil.NSError

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

func validUTF8CString(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, '\x00')
}
