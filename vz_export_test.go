package vz

import (
	"runtime"
)

func (v *VirtualMachine) SetMachineStateFinalizer(f func()) {
	runtime.SetFinalizer(v.stateHandle, func(self *machineState) {
		f()
	})
}
