package vz

import (
	"fmt"
	"os"
)

// CreateDiskImage is creating disk image with specified filename and filesize.
// For example, if you want to create disk with 64GiB, you can set "64 * 1024 * 1024 * 1024" to size.
func CreateDiskImage(name string, size int64) error {
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to create disk image: %w", err)
	}
	defer f.Close()

	if err := f.Truncate(size); err != nil {
		return fmt.Errorf("failed to truncate: %w", err)
	}
	return nil
}
