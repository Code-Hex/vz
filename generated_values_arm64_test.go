//go:build darwin && arm64

package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/vzbridge"
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
			call            func() int64
		}{
			{"VZLinuxRosettaAbstractSocketCachingOptions", "maximumNameLength", vzbridge.Framework_VZLinuxRosettaDirectoryShare_CachingOptions_maximumNameLength_d18d70a5},
			{"VZLinuxRosettaUnixSocketCachingOptions", "maximumPathLength", vzbridge.Framework_VZLinuxRosettaDirectoryShare_CachingOptions_maximumPathLength_d9e4ff7a},
		} {
			got := tt.call()
			want := pureobjc.Send[uint64](pureobjc.ID(pureobjc.GetClass(tt.class)), pureobjc.RegisterName(tt.selector))
			if uint64(got) != want {
				t.Errorf("%s: got %d, want %d", tt.selector, got, want)
			}
		}
	}
}
