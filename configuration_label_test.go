package vz_test

import (
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestVirtualMachineLabel(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	boot, err := vz.NewEFIBootLoader()
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"", " ", strings.Repeat("a", 65), "a\x00b", "\xff"} {
		if _, err := vz.NewVirtualMachineConfiguration(boot, 1, 1024*1024*1024, vz.WithVirtualMachineLabel(label)); err == nil {
			t.Fatalf("accepted label %q", label)
		}
	}
	for _, label := range []string{"Build VM", strings.Repeat("😀", 64)} {
		config, err := vz.NewVirtualMachineConfiguration(boot, 1, 1024*1024*1024, vz.WithVirtualMachineLabel(label))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := config.Label(); err != nil || got != label {
			t.Fatalf("label = %q, error = %v", got, err)
		}
	}
}
