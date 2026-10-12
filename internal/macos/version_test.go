package macos_test

import (
	"errors"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/macos"
)

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  float64
	}{
		{"11", 11},
		{"12.0", 12},
		{"12.3.1", 12.3},
		{"26.1.2", 26.1},
		{"27.0.1", 27},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := macos.ParseVersion(tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("ParseVersion(%q) = %v, %v; want %v, nil", tc.input, got, err, tc.want)
			}
		})
	}
	for _, input := range []string{"", "unknown", "v26.0", "26.0.1.2"} {
		t.Run(input, func(t *testing.T) {
			if _, err := macos.ParseVersion(input); err == nil {
				t.Fatalf("ParseVersion(%q) accepted an invalid OS version", input)
			}
		})
	}
}

func TestBuildTargetAvailable(t *testing.T) {
	for _, tc := range []struct {
		version float64
		sdk     int
	}{
		{11, 110000}, {12, 120000}, {12.3, 120300}, {13, 130000},
		{14, 140000}, {15, 150000}, {26, 260000},
	} {
		if err := macos.BuildTargetAvailable(tc.version, tc.sdk); err != nil {
			t.Errorf("BuildTargetAvailable(%v, %v) = %v", tc.version, tc.sdk, err)
		}
		if err := macos.BuildTargetAvailable(tc.version, tc.sdk-1); !errors.Is(err, macos.ErrBuildTargetOSVersion) {
			t.Errorf("BuildTargetAvailable(%v, %v) = %v, want build-target error", tc.version, tc.sdk-1, err)
		}
	}
	for _, tc := range []struct {
		sdk  int
		want string
	}{
		{0, "undefined __MAC_OS_X_VERSION_MAX_ALLOWED: unsupported build target macOS version"},
		{150000, "unsupported build target macOS version for 26.0 (the binary was built with __MAC_OS_X_VERSION_MAX_ALLOWED=150000; needs recompilation)"},
	} {
		err := macos.BuildTargetAvailable(26, tc.sdk)
		if err == nil || err.Error() != tc.want {
			t.Errorf("BuildTargetAvailable(26, %v) = %v, want %q", tc.sdk, err, tc.want)
		}
	}
}
