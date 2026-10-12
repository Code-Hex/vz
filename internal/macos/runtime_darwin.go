package macos

import (
	"sync"
	"syscall"
)

var currentVersion = sync.OnceValues(func() (float64, error) {
	osver, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return 0, err
	}
	return ParseVersion(osver)
})

// Available checks the running macOS version and the build SDK.
func Available(version float64, allowedVersion int) error {
	current, err := currentVersion()
	if err != nil {
		return err
	}
	if current < version {
		return ErrUnsupportedOSVersion
	}
	return BuildTargetAvailable(version, allowedVersion)
}
