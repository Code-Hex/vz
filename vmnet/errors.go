package vmnet

import (
	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
)

// Error contains the details of an error returned by vmnet.framework.
type Error = objcutil.NSError

var (
	// ErrUnsupportedOSVersion is returned when an API requires a newer macOS version.
	ErrUnsupportedOSVersion = macos.ErrUnsupportedOSVersion

	// ErrBuildTargetOSVersion is returned when the build SDK does not include an API.
	ErrBuildTargetOSVersion = macos.ErrBuildTargetOSVersion
)
