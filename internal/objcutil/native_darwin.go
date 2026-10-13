package objcutil

import (
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// NewNSError consumes an owned NSError reference and copies its details.
func NewNSError(p unsafe.Pointer) *NSError {
	if p == nil {
		return nil
	}
	object := objc.NewManagedPointer(p, vzbridge.ReleaseObject)
	defer objc.Release(object)
	info := vzbridge.NSError_UserInfo(object)
	defer objc.Release(info)
	return &NSError{
		Domain:               String(vzbridge.NSError_Domain(object)),
		Code:                 int(vzbridge.NSError_Code(object)),
		LocalizedDescription: String(vzbridge.NSError_LocalizedDescription(object)),
		UserInfo:             String(vzbridge.NSObject_Description(info)),
	}
}

// String consumes an owned NSString reference and copies its UTF-8 contents.
func String(object *objc.Pointer) string {
	if object == nil {
		return ""
	}
	defer objc.Release(object)
	return string(Bytes(vzbridge.NSString_DataUsingEncoding(object, vzbridge.NSUTF8StringEncoding)))
}

// Bytes consumes an owned NSData reference and copies its contents.
func Bytes(object *objc.Pointer) []byte {
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
