package vz

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -lobjc -framework Foundation
#import <Foundation/Foundation.h>
*/
import "C"
import (
	"unsafe"

	"github.com/Code-Hex/vz/v2/internal/objc"
)

type pointer = objc.Pointer
type NSError = objc.NSError

// CharWithGoString makes *Char which is *C.Char wrapper from Go string.
func charWithGoString(s string) *char {
	return (*char)(unsafe.Pointer(C.CString(s)))
}

// Char is a wrapper of C.char
type char C.char

// CString converts *C.char from *Char
func (c *char) CString() *C.char {
	return (*C.char)(c)
}

// String converts Go string from *Char
func (c *char) String() string {
	return C.GoString((*C.char)(c))
}

// Free frees allocated *C.char in Go code
func (c *char) Free() {
	C.free(unsafe.Pointer(c))
}
