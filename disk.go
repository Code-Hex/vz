package vz

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// CreateDiskImage is creating disk image with specified filename and filesize.
// For example, if you want to create disk with 64GiB, you can set "64 * 1024 * 1024 * 1024" to size.
//
// Note that if you have specified a pathname which already exists, this function
// returns os.ErrExist error. So you can handle it with os.IsExist function.
func CreateDiskImage(pathname string, size int64) error {
	f, err := os.OpenFile(pathname, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Truncate(size); err != nil {
		return err
	}
	return nil
}

// CreateSparseDiskImage is creating an "Apple Sparse Image Format" disk image
// with specified filename and filesize. The function "shells out" to diskutil, as currently
// this is the only known way of creating ASIF images.
// For example, if you want to create disk with 64GiB, you can set "64 * 1024 * 1024 * 1024" to size.
//
// Note that ASIF is only available from macOS Tahoe, so the function will return error
// on earlier versions.
func CreateSparseDiskImage(ctx context.Context, pathname string, size int64) error {
	if err := macOSAvailable(26); err != nil {
		return err
	}
	diskutil, err := exec.LookPath("diskutil")
	if err != nil {
		return fmt.Errorf("failed to find disktuil: %w", err)
	}

	sizeStr := strconv.FormatInt(size, 10)
	cmd := exec.CommandContext(ctx,
		diskutil,
		"image",
		"create",
		"blank",
		"--fs", "none",
		"--format", "ASIF",
		"--size", sizeStr,
		pathname)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create ASIF disk image: %w", err)
	}

	return nil
}
