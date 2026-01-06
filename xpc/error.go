package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"
import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// RichError represents an XPC rich error. ([XPC_TYPE_RICH_ERROR]) [TypeRichError]
//
// [XPC_TYPE_RICH_ERROR]: https://developer.apple.com/documentation/xpc/xpc_rich_error_t?language=objc
type RichError struct {
	*xpcObject
}

var _ Object = &RichError{}

var _ error = RichError{}

// newRichError creates a new RichError from an existing xpc_rich_error_t.
// internal use only.
func newRichError(richErr unsafe.Pointer) *RichError {
	if richErr == nil {
		return nil
	}
	return &RichError{newXpcObject(richErr)}
}

// CanRetry indicates whether the operation that caused the [RichError] can be retried.
//
//   - https://developer.apple.com/documentation/xpc/xpc_rich_error_can_retry(_:)?language=objc
func (e RichError) CanRetry() bool {
	return bool(C.xpcRichErrorCanRetry(objc.Ptr(e)))
}

// Error implements the [error] interface.
//
//   - https://developer.apple.com/documentation/xpc/xpc_rich_error_copy_description(_:)?language=objc
func (e RichError) Error() string {
	desc := C.xpcRichErrorCopyDescription(objc.Ptr(e))
	defer C.free(unsafe.Pointer(desc))
	return C.GoString(desc)
}
