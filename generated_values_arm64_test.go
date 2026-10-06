//go:build darwin && arm64

package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestGeneratedRosettaValueMigration(t *testing.T) {
	if macOSAvailable(13) == nil {
		got := LinuxRosettaDirectoryShareAvailability()
		want := pureobjc.Send[int64](pureobjc.ID(pureobjc.GetClass("VZLinuxRosettaDirectoryShare")), pureobjc.RegisterName("availability"))
		if int64(got) != want {
			t.Errorf("Rosetta availability: got %d, want %d", got, want)
		}
	}
	if macOSAvailable(14) == nil {
		for _, tt := range []struct {
			class, selector string
			call            func() uint64
		}{
			{"VZLinuxRosettaAbstractSocketCachingOptions", "maximumNameLength", vzbridge.VZLinuxRosettaAbstractSocketCachingOptions_MaximumNameLength},
			{"VZLinuxRosettaUnixSocketCachingOptions", "maximumPathLength", vzbridge.VZLinuxRosettaUnixSocketCachingOptions_MaximumPathLength},
		} {
			got := tt.call()
			want := pureobjc.Send[uint64](pureobjc.ID(pureobjc.GetClass(tt.class)), pureobjc.RegisterName(tt.selector))
			if got != want {
				t.Errorf("%s: got %d, want %d", tt.selector, got, want)
			}
		}
	}
}
