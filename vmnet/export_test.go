package vmnet

import (
	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func Available(version float64) bool {
	return macos.Available(version, vzbridge.SDKVersion) != nil
}
