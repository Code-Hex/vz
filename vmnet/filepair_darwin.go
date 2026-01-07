package vmnet

// # include <net/ethernet.h>
import "C"
import (
	"fmt"
	"os"
	"syscall"
)

// filePair creates a pair of connected *[os.File] using [syscall.Socketpair].
func filePair(typ, sendBufSize, recvBufSize int) (connFile, passingFile *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, typ, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create socketpair: %w", err)
	}
	connFd, passingFd := fds[0], fds[1]
	// Set up connFd.
	if err := setupFd(connFd, sendBufSize, recvBufSize); err != nil {
		_ = syscall.Close(connFd)
		_ = syscall.Close(passingFd)
		return nil, nil, err
	}
	// Set up passingFd.
	if err := setupFd(passingFd, sendBufSize, recvBufSize); err != nil {
		_ = syscall.Close(connFd)
		_ = syscall.Close(passingFd)
		return nil, nil, err
	}
	connFile = os.NewFile(uintptr(connFd), "vmnet-conn-file")
	passingFile = os.NewFile(uintptr(passingFd), "vmnet-passing-file")
	return connFile, passingFile, nil
}

// setupFd sets non-blocking and buffer sizes on the given fd.
func setupFd(fd, sendBufSize, recvBufSize int) error {
	if err := syscall.SetNonblock(fd, true); err != nil {
		return fmt.Errorf("failed to set nonblock on fd: %w", err)
	}
	// Default SNDLOWAT is 2048 bytes, which is too large for our use case.
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDLOWAT, int(headerSize)+C.ETHER_MIN_LEN); err != nil {
		return fmt.Errorf("failed to set SO_SNDLOWAT on fd: %w", err)
	}
	if recvBufSize > 0 {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, recvBufSize); err != nil {
			return fmt.Errorf("failed to set SO_RCVBUF on fd: %w", err)
		}
	}
	if sendBufSize > 0 {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, sendBufSize); err != nil {
			return fmt.Errorf("failed to set SO_SNDBUF on fd: %w", err)
		}
	}
	return nil
}
