//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package diskimagekit

import (
	"errors"
	"os"
)

func lockFile(file *os.File, mode OpenMode) error {
	return errors.New("diskimagekit: advisory file locks are not supported on this operating system")
}

func openRegularFile(path string, flags int) (*os.File, error) {
	return nil, errors.New("diskimagekit: file access is not supported on this operating system")
}
