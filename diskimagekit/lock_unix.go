//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package diskimagekit

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(file *os.File, mode OpenMode) error {
	operation := syscall.LOCK_SH
	if mode == ReadWrite {
		operation = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(file.Fd()), operation|syscall.LOCK_NB); err != nil {
		return &os.PathError{
			Op:   "flock",
			Path: file.Name(),
			Err:  err,
		}
	}
	return nil
}

func openRegularFile(path string, flags int) (*os.File, error) {
	// A FIFO can block in OpenFile before we get a chance to reject its type.
	f, err := os.OpenFile(path, flags|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("diskimagekit: image must be a regular file")
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
