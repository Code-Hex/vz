package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
# include "xpc_darwin.h"
*/
import "C"

// Type represents an XPC type (xpc_type_t).
//   - https://developer.apple.com/documentation/xpc/xpc_type_t?language=objc
type Type struct {
	xpcType C.xpc_type_t
}

var (
	TypeActivity   = Type{C.XPC_TYPE_ACTIVITY}   // https://developer.apple.com/documentation/xpc/xpc_type_activity-c.macro?language=objc
	TypeArray      = Type{C.XPC_TYPE_ARRAY}      // https://developer.apple.com/documentation/xpc/xpc_type_array-c.macro?language=objc
	TypeBool       = Type{C.XPC_TYPE_BOOL}       // https://developer.apple.com/documentation/xpc/xpc_type_bool-c.macro?language=objc
	TypeConnection = Type{C.XPC_TYPE_CONNECTION} // https://developer.apple.com/documentation/xpc/xpc_type_connection-c.macro?language=objc
	TypeData       = Type{C.XPC_TYPE_DATA}       // https://developer.apple.com/documentation/xpc/xpc_type_data-c.macro?language=objc
	TypeDate       = Type{C.XPC_TYPE_DATE}       // https://developer.apple.com/documentation/xpc/xpc_type_date-c.macro?language=objc
	TypeDictionary = Type{C.XPC_TYPE_DICTIONARY} // https://developer.apple.com/documentation/xpc/xpc_type_dictionary-c.macro?language=objc
	TypeDouble     = Type{C.XPC_TYPE_DOUBLE}     // https://developer.apple.com/documentation/xpc/xpc_type_double-c.macro?language=objc
	TypeEndpoint   = Type{C.XPC_TYPE_ENDPOINT}   // https://developer.apple.com/documentation/xpc/xpc_type_endpoint-c.macro?language=objc
	TypeError      = Type{C.XPC_TYPE_ERROR}      // https://developer.apple.com/documentation/xpc/xpc_type_error-c.macro?language=objc
	TypeFD         = Type{C.XPC_TYPE_FD}         // https://developer.apple.com/documentation/xpc/xpc_type_fd-c.macro?language=objc
	TypeInt64      = Type{C.XPC_TYPE_INT64}      // https://developer.apple.com/documentation/xpc/xpc_type_int64-c.macro?language=objc
	TypeNull       = Type{C.XPC_TYPE_NULL}       // https://developer.apple.com/documentation/xpc/xpc_type_null-c.macro?language=objc
	TypeRichError  = Type{C.XPC_TYPE_RICH_ERROR} // does not have official documentation, but defined in <xpc.h>
	TypeSession    = Type{C.XPC_TYPE_SESSION}    // does not have official documentation, but defined in <xpc/session.h>
	TypeShmem      = Type{C.XPC_TYPE_SHMEM}      // https://developer.apple.com/documentation/xpc/xpc_type_shmem-c.macro?language=objc
	TypeString     = Type{C.XPC_TYPE_STRING}     // https://developer.apple.com/documentation/xpc/xpc_type_string-c.macro?language=objc
	TypeUInt64     = Type{C.XPC_TYPE_UINT64}     // https://developer.apple.com/documentation/xpc/xpc_type_uint64-c.macro?language=objc
	TypeUUID       = Type{C.XPC_TYPE_UUID}       // https://developer.apple.com/documentation/xpc/xpc_type_uuid-c.macro?language=objc
)

// String returns the name of the [XpcType].
// see: https://developer.apple.com/documentation/xpc/xpc_type_get_name(_:)?language=objc
func (t Type) String() string {
	cs := C.xpc_type_get_name(t.xpcType)
	if cs == nil {
		return "<unknown>"
	}
	// do not free cs since it is managed by XPC runtime.
	return C.GoString(cs)
}
