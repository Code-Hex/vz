//go:build darwin && arm64
// +build darwin,arm64

package vz

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_14_arm64.h"
*/
import "C"
import "github.com/Code-Hex/vz/v3/internal/objc"

func (v *VirtualMachineConfiguration) ValidateSaveRestoreSupport() (bool, error) {
	nserrPtr := newNSErrorAsNil()
	ret := C.validateSaveRestoreSupportWithError(objc.Ptr(v), &nserrPtr)
	err := newNSError(nserrPtr)
	if err != nil {
		return false, err
	}
	return (bool)(ret), nil
}
