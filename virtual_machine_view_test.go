//go:build darwin

package vz

import "testing"

func TestVirtualMachineViewRejectsInvalidMachine(t *testing.T) {
	for _, machine := range []*VirtualMachine{nil, {}} {
		if view, err := NewVirtualMachineView(machine); view != nil || err == nil {
			t.Fatalf("NewVirtualMachineView = %v, %v", view, err)
		}
	}
}
