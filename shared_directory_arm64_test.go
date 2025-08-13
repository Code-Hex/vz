//go:build darwin && arm64
// +build darwin,arm64

package vz_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v3"
)

func TestLinuxRosettaAvailabilityString(t *testing.T) {
	cases := []struct {
		availability vz.LinuxRosettaAvailability
		want         string
	}{
		{
			availability: vz.LinuxRosettaAvailabilityNotSupported,
			want:         "LinuxRosettaAvailabilityNotSupported",
		},
		{
			availability: vz.LinuxRosettaAvailabilityNotInstalled,
			want:         "LinuxRosettaAvailabilityNotInstalled",
		},
		{
			availability: vz.LinuxRosettaAvailabilityInstalled,
			want:         "LinuxRosettaAvailabilityInstalled",
		},
	}
	for _, tc := range cases {
		got := tc.availability.String()
		if tc.want != got {
			t.Fatalf("want %q but got %q", tc.want, got)
		}
	}
}

func TestNewLinuxRosettaUnixSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaUnixSocketCachingOptions is supported from macOS 14")
	}
	dir := t.TempDir()
	t.Run("invalid filename length", func(t *testing.T) {
		filename := filepath.Join(dir, strings.Repeat("a", 150)) + ".txt"
		f, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		_, err = vz.NewLinuxRosettaUnixSocketCachingOptions(filename)
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); !strings.Contains(got, "maximum allowed length of") {
			t.Fatalf("unexpected error: %q", got)
		}
	})
}

func TestNewLinuxRosettaAbstractSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaAbstractSocketCachingOptions is supported from macOS 14")
	}
	t.Run("invalid name length", func(t *testing.T) {
		name := strings.Repeat("a", 350)
		_, err := vz.NewLinuxRosettaAbstractSocketCachingOptions(name)
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); !strings.Contains(got, "maximum allowed length of") {
			t.Fatalf("unexpected error: %q", got)
		}
	})
}
