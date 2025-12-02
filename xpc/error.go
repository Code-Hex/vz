package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"unsafe"
)

// RichError represents an XPC rich error. ([XPC_TYPE_RICH_ERROR]) [TypeRichError]
//
// [XPC_TYPE_RICH_ERROR]: https://developer.apple.com/documentation/xpc/xpc_rich_error_t?language=objc
type RichError struct {
	*XpcObject
}

var _ Object = &RichError{}

var _ error = RichError{}

// newRichError creates a new RichError from an existing xpc_rich_error_t.
// internal use only.
func newRichError(richErr unsafe.Pointer) *RichError {
	if richErr == nil {
		return nil
	}
	return &RichError{XpcObject: &XpcObject{richErr}}
}

// CanRetry indicates whether the operation that caused the [RichError] can be retried.
//
//   - https://developer.apple.com/documentation/xpc/xpc_rich_error_can_retry(_:)?language=objc
func (e RichError) CanRetry() bool {
	return bool(C.xpcRichErrorCanRetry(e.Raw()))
}

// Error implements the [error] interface.
//
//   - https://developer.apple.com/documentation/xpc/xpc_rich_error_copy_description(_:)?language=objc
func (e RichError) Error() string {
	desc := C.xpcRichErrorCopyDescription(e.Raw())
	defer C.free(unsafe.Pointer(desc))
	return C.GoString(desc)
}
