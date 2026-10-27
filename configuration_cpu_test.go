package vz_test

import (
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestVirtualMachineConfigurationCPUCount(t *testing.T) {
	if vz.Available(12) {
		t.Skip("requires macOS 12")
	}
	store, err := vz.NewEFIVariableStore(filepath.Join(t.TempDir(), "efi"), vz.WithCreatingEFIVariableStore())
	if err != nil {
		t.Fatal(err)
	}
	boot, err := vz.NewEFIBootLoader(vz.WithEFIVariableStore(store))
	if err != nil {
		t.Fatal(err)
	}
	minimumCPUCount := vz.VirtualMachineConfigurationMinimumAllowedCPUCount()
	for _, tc := range []struct {
		name  string
		cpu   uint
		valid bool
	}{
		{name: "minimum", cpu: minimumCPUCount, valid: true},
		{name: "exceeds 32 bits", cpu: 1<<32 + minimumCPUCount, valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := vz.NewVirtualMachineConfiguration(boot, tc.cpu, vz.VirtualMachineConfigurationMinimumAllowedMemorySize())
			if err != nil {
				t.Fatal(err)
			}
			valid, err := config.Validate()
			if valid != tc.valid || (err == nil) != tc.valid {
				t.Fatalf("CPU count %d: Validate() = %t, %v; want valid = %t", tc.cpu, valid, err, tc.valid)
			}
		})
	}
}
