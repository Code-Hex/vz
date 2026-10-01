//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

// ValidateSaveRestoreSupport Determines whether the framework can save or restore the VM’s current configuration.
//
// Verify that a virtual machine with this configuration is savable.
// Not all configuration options can be safely saved and restored from file.
//
// If this evaluates to false, the caller should expect future calls to `(*VirtualMachine).SaveMachineStateToPath` to fail.
// error If not nil, assigned with an error describing the unsupported configuration option.
func (v *VirtualMachineConfiguration) ValidateSaveRestoreSupport() (bool, error) {
	var nserrPtr unsafe.Pointer
	ret := vzbridge.VZVirtualMachineConfiguration_ValidateSaveRestoreSupportWithError(v, &nserrPtr)
	err := newNSError(nserrPtr)
	if err != nil {
		return false, err
	}
	return (bool)(ret), nil
}
