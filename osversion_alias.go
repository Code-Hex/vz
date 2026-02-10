package vz

import "github.com/Code-Hex/vz/v3/internal/osversion"

var (
	// ErrUnsupportedOSVersion is returned when calling a method which is only
	// available in newer macOS versions.
	ErrUnsupportedOSVersion = osversion.ErrUnsupportedOSVersion

	// ErrBuildTargetOSVersion indicates that the API is available but the
	// running program has disabled it.
	ErrBuildTargetOSVersion = osversion.ErrBuildTargetOSVersion

	macOSAvailable = osversion.MacOSAvailable

	// MacOSBuildTargetAvailable checks whether the API available in a given version has been compiled.
	macOSBuildTargetAvailable = osversion.MacOSBuildTargetAvailable
)

// for Testing
var (
	fetchMajorMinorVersion = osversion.FetchMajorMinorVersion
	majorMinorVersion      = &osversion.MajorMinorVersion
	majorMinorVersionOnce  = &osversion.MajorMinorVersionOnce
	maxAllowedVersion      = &osversion.MaxAllowedVersion
	maxAllowedVersionOnce  = &osversion.MaxAllowedVersionOnce
	sysctl                 = &osversion.Sysctl
)
