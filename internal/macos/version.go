package macos

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	// ErrUnsupportedOSVersion is returned when calling a method which is only
	// available in newer macOS versions.
	ErrUnsupportedOSVersion = errors.New("unsupported macOS version")

	// ErrBuildTargetOSVersion indicates that the API is available but the
	// running program has disabled it.
	ErrBuildTargetOSVersion = errors.New("unsupported build target macOS version")
)

// ParseVersion returns the major and minor macOS version.
func ParseVersion(osver string) (float64, error) {
	prefix := "v"
	majorMinor := strings.TrimPrefix(semver.MajorMinor(prefix+osver), prefix)
	version, err := strconv.ParseFloat(majorMinor, 64)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// BuildTargetAvailable checks whether the SDK includes an API.
func BuildTargetAvailable(version float64, allowedVersion int) error {
	if allowedVersion == 0 {
		return fmt.Errorf("undefined __MAC_OS_X_VERSION_MAX_ALLOWED: %w", ErrBuildTargetOSVersion)
	}

	// This list from AvailabilityVersions.h
	var target int
	switch version {
	case 11:
		target = 110000 // __MAC_11_0
	case 12:
		target = 120000 // __MAC_12_0
	case 12.3:
		target = 120300 // __MAC_12_3
	case 13:
		target = 130000 // __MAC_13_0
	case 14:
		target = 140000 // __MAC_14_0
	case 15:
		target = 150000 // __MAC_15_0
	case 26:
		target = 260000 // __MAC_26_0
	}
	if allowedVersion < target {
		return fmt.Errorf("%w for %.1f (the binary was built with __MAC_OS_X_VERSION_MAX_ALLOWED=%d; needs recompilation)",
			ErrBuildTargetOSVersion, version, allowedVersion)
	}
	return nil
}
