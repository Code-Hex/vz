package vz

import (
	"runtime"
)

func (v *VirtualMachine) SetMachineStateFinalizer(f func()) {
	runtime.SetFinalizer(v.machineState, func(self *machineState) {
		f()
	})
}
