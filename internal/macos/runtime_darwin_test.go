package macos_test

import (
	"errors"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/macos"
)

func TestAvailable(t *testing.T) {
	if err := macos.Available(0, 270000); err != nil {
		t.Fatalf("current macOS is unavailable: %v", err)
	}
	if err := macos.Available(999, 270000); !errors.Is(err, macos.ErrUnsupportedOSVersion) {
		t.Fatalf("future macOS requirement returned %v, want unsupported OS", err)
	}
	if err := macos.Available(0, 0); !errors.Is(err, macos.ErrBuildTargetOSVersion) {
		t.Fatalf("missing SDK returned %v, want unsupported build target", err)
	}
}
