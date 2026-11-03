//go:build darwin

package vzbridge

/*
#include "virtual_machine_view.h"
*/
import "C"
import "unsafe"

func ViewIsMainThread() bool                        { return bool(C.vz_viewIsMainThread()) }
func ViewNew(machine unsafe.Pointer) unsafe.Pointer { return C.vz_viewNew(machine) }
func ViewRelease(view unsafe.Pointer)               { C.vz_viewRelease(view) }
func ViewCapturesKeys(view unsafe.Pointer) bool     { return bool(C.vz_viewCapturesKeys(view)) }
func ViewSetCapturesKeys(view unsafe.Pointer, value bool) {
	C.vz_viewSetCapturesKeys(view, C.bool(value))
}
func ViewAutomaticallyReconfigures(view unsafe.Pointer) bool {
	return bool(C.vz_viewAutomaticallyReconfigures(view))
}
func ViewSetAutomaticallyReconfigures(view unsafe.Pointer, value bool) {
	C.vz_viewSetAutomaticallyReconfigures(view, C.bool(value))
}

func ViewBorrowPoolNew() unsafe.Pointer       { return C.vz_viewBorrowPoolNew() }
func ViewBorrowPoolDrain(pool unsafe.Pointer) { C.vz_viewBorrowPoolDrain(pool) }
