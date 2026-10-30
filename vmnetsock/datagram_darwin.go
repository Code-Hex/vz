package vmnetsock

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// readDatagramPackets waits for the first packet, then drains queued packets.
// Frames borrow buffer until the next call. An invalid packet returns the valid
// prefix and an error together.
func readDatagramPackets(raw syscall.RawConn, frames [][]byte, buffer []byte, maximum int) (int, error) {
	var count, used int
	var recvErr error
	err := raw.Read(func(fd uintptr) bool {
		for count < len(frames) && len(buffer)-used >= maximum {
			frame := buffer[used : used+maximum]
			n, _, flags, _, err := unix.Recvmsg(int(fd), frame, nil, unix.MSG_DONTWAIT)
			switch err {
			case nil:
			case unix.EINTR:
				continue
			case unix.EAGAIN:
				// Waiting after a partial batch would delay packets already read.
				return count > 0
			default:
				recvErr = os.NewSyscallError("recvmsg", err)
				return true
			}
			if flags&unix.MSG_TRUNC != 0 {
				recvErr = fmt.Errorf("datagram frame exceeds maximum %d", maximum)
				return true
			}
			if n == 0 {
				recvErr = fmt.Errorf("empty datagram frame")
				return true
			}
			frames[count] = frame[:n:n]
			used += n
			count++
		}
		return true
	})
	if err != nil {
		return count, err
	}
	return count, recvErr
}

func writeDatagramPackets(raw syscall.RawConn, frames [][]byte) error {
	var sent int
	var sendErr error
	err := raw.Write(func(fd uintptr) bool {
		for sent < len(frames) {
			frame := frames[sent]
			n, err := unix.SendmsgN(int(fd), frame, nil, nil, unix.MSG_DONTWAIT)
			switch err {
			case nil:
				if n != len(frame) {
					sendErr = io.ErrShortWrite
					return true
				}
				sent++
			case unix.EINTR:
				continue
			case unix.EAGAIN, unix.ENOBUFS:
				// Darwin wakes connected datagram senders when the peer drains its
				// receive queue. Preserve sent so the accepted prefix is not repeated.
				// https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/uipc_usrreq.c#L425-L478
				return false
			default:
				sendErr = os.NewSyscallError("sendmsg", err)
				return true
			}
		}
		return true
	})
	if err != nil {
		return err
	}
	return sendErr
}
