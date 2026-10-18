package vmnetsock

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func socketPair(transport Transport, maxSize int) (*net.UnixConn, *os.File, error) {
	kind := unix.SOCK_STREAM
	if transport == Datagram {
		kind = unix.SOCK_DGRAM
	}

	syscall.ForkLock.RLock()
	fds, err := unix.Socketpair(unix.AF_UNIX, kind, 0)
	if err == nil {
		unix.CloseOnExec(fds[0])
		unix.CloseOnExec(fds[1])
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, nil, err
	}

	local := os.NewFile(uintptr(fds[0]), "vmnet-bridge")
	peer := os.NewFile(uintptr(fds[1]), "vmnet-guest")
	fail := func(err error) (*net.UnixConn, *os.File, error) {
		_ = local.Close()
		_ = peer.Close()
		return nil, nil, err
	}

	size := max(256*1024, 2*(maxSize+4))
	for _, fd := range fds {
		for _, option := range []int{unix.SO_SNDBUF, unix.SO_RCVBUF} {
			if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, option, size); err != nil {
				return fail(err)
			}
		}
	}

	c, err := net.FileConn(local)
	if err != nil {
		return fail(err)
	}
	if err := local.Close(); err != nil {
		_ = c.Close()
		_ = peer.Close()
		return nil, nil, err
	}
	return c.(*net.UnixConn), peer, nil
}
