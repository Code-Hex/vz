//go:build darwin && arm64
// +build darwin,arm64

package vz_test

import (
	"testing"

	"github.com/Code-Hex/vz/v2"
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
