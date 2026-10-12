package vz

import (
	"sync"
	"syscall"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

var (
	// ErrUnsupportedOSVersion is returned when calling a method which is only
	// available in newer macOS versions.
	ErrUnsupportedOSVersion = macos.ErrUnsupportedOSVersion

	// ErrBuildTargetOSVersion indicates that the API is available but the
	// running program has disabled it.
	ErrBuildTargetOSVersion = macos.ErrBuildTargetOSVersion
)

func macOSAvailable(version float64) error {
	if macOSMajorMinorVersion() < version {
		return ErrUnsupportedOSVersion
	}
	return macOSBuildTargetAvailable(version, vzbridge.SDKVersion)
}

var (
	majorMinorVersion     float64
	majorMinorVersionOnce sync.Once

	// This can be replaced in the test code to enable mock.
	// It will not be changed in production.
	sysctl = syscall.Sysctl
)

func fetchMajorMinorVersion() (float64, error) {
	osver, err := sysctl("kern.osproductversion")
	if err != nil {
		return 0, err
	}
	return macos.ParseVersion(osver)
}

func macOSMajorMinorVersion() float64 {
	majorMinorVersionOnce.Do(func() {
		version, err := fetchMajorMinorVersion()
		if err != nil {
			panic(err)
		}
		majorMinorVersion = version
	})
	return majorMinorVersion
}

// macOSBuildTargetAvailable checks whether the API available in a given version has been compiled.
func macOSBuildTargetAvailable(version float64, allowedVersion int) error {
	return macos.BuildTargetAvailable(version, allowedVersion)
}
