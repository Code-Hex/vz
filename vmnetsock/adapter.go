package vmnetsock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/Code-Hex/vz/v4/vmnet"
	"golang.org/x/sys/unix"
)

// Transport specifies the framing protocol used on the socket descriptor.
type Transport uint8

const (
	// Stream uses a four-byte big-endian packet length followed by packet bytes.
	Stream Transport = iota

	// Datagram sends one packet per UNIX datagram.
	Datagram
)

// Adapter forwards packets until Close, context cancellation, or a forwarding error.
// Copies share a single lifetime. Adapter borrows the Interface and never calls Stop.
type Adapter struct {
	state *adapterState
}

type adapterState struct {
	parent context.Context
	cancel context.CancelFunc
	conn   *net.UnixConn
	file   *os.File
	once   sync.Once
	done   chan struct{}
	err    error
}

// New creates a socket pair and starts forwarding immediately.
// The Adapter owns both sockets and borrows iface without stopping it.
// Dup returns a caller-owned socket to pass through exec.Cmd.ExtraFiles.
// Packets pass through unchanged. If iface uses Virtio headers, the peer must
// support that format and the offload features enabled on iface.
func New(ctx context.Context, iface *vmnet.Interface, transport Transport) (*Adapter, error) {
	if iface == nil || iface.MaxPacketSize() == 0 {
		return nil, fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	if transport != Stream && transport != Datagram {
		return nil, fmt.Errorf("invalid vmnetsock transport %d", transport)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, file, err := socketPair(transport, iface.MaxPacketSize())
	if err != nil {
		return nil, err
	}

	run, cancel := context.WithCancel(ctx)
	s := &adapterState{
		parent: ctx,
		cancel: cancel,
		conn:   conn,
		file:   file,
		done:   make(chan struct{}),
	}

	var pumps sync.WaitGroup
	pumps.Go(func() {
		s.finish(s.forwardToInterface(run, iface, transport))
	})
	pumps.Go(func() {
		s.finish(s.forwardToPeer(run, iface, transport))
	})
	go func() {
		<-run.Done()
		s.finish(run.Err())
		pumps.Wait()
		close(s.done)
	}()

	return &Adapter{state: s}, nil
}

// Dup returns a new, caller-owned file descriptor for the peer socket.
// The caller must close the returned file independently of Adapter.Close.
// Closing duplicates does not stop forwarding. Calling Dup after Close returns os.ErrClosed.
func (a *Adapter) Dup() (*os.File, error) {
	if a == nil || a.state == nil {
		return nil, os.ErrClosed
	}
	raw, err := a.state.file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var fd int
	var dupErr error
	if err := raw.Control(func(original uintptr) {
		fd, dupErr = unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0)
	}); err != nil {
		return nil, os.ErrClosed
	}
	if dupErr != nil {
		return nil, os.NewSyscallError("fcntl", dupErr)
	}
	return os.NewFile(uintptr(fd), "vmnet-peer"), nil
}

func (s *adapterState) forwardToInterface(ctx context.Context, iface *vmnet.Interface, transport Transport) error {
	var readPackets func() ([][]byte, error)
	if transport == Stream {
		reader := newStreamPacketReader(s.conn, iface.MaxPacketSize())
		readPackets = reader.readPackets
	} else {
		size := min(iface.MaxPacketSize(), packetBatchBytes)
		count := packetBatchSize(size)
		raw, err := s.conn.SyscallConn()
		if err != nil {
			return err
		}
		frames := make([][]byte, count)
		for i := range frames {
			frames[i] = make([]byte, size)
		}
		readPackets = func() ([][]byte, error) {
			n, err := readDatagramPackets(raw, frames, size)
			return frames[:n], err
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frames, err := readPackets()
		if len(frames) > 0 {
			if _, writeErr := iface.WritePackets(frames); writeErr != nil && !vmnetBufferExhausted(writeErr) {
				return writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *adapterState) forwardToPeer(ctx context.Context, iface *vmnet.Interface, transport Transport) error {
	size := iface.MaxPacketSize()
	count := packetBatchSize(size)
	frames := make([][]byte, count)
	for j := range frames {
		frames[j] = make([]byte, size)
	}
	var headers [][4]byte
	var vectors net.Buffers
	var raw syscall.RawConn
	if transport == Stream {
		headers = make([][4]byte, count)
		vectors = make(net.Buffers, 2*count)
	} else {
		var err error
		raw, err = s.conn.SyscallConn()
		if err != nil {
			return err
		}
	}
	for {
		n, err := iface.ReadPackets(ctx, frames)
		if err != nil {
			return err
		}
		if transport == Stream {
			if err := writeStreamFrames(s.conn, frames[:n], headers, vectors); err != nil {
				return err
			}
		} else {
			// Drop the unsent suffix on ENOBUFS. Socket writability does not
			// guarantee space in the peer's datagram receive buffer.
			if err := writeDatagramPackets(raw, frames[:n]); err != nil && !errors.Is(err, unix.ENOBUFS) {
				return err
			}
		}
	}
}

func (s *adapterState) finish(err error) {
	s.once.Do(func() {
		if s.parent.Err() != nil {
			err = s.parent.Err()
		}
		s.err = err
		s.cancel()
		_ = s.conn.Close()
		_ = s.file.Close()
	})
}

// Close stops both forwarding directions and waits for them to finish.
// It closes its own sockets without closing duplicates or stopping the borrowed Interface.
// Stream peer EOF and an explicit Close return nil if they cause termination first.
// Earlier failures and cancellation remain the result of subsequent calls.
func (a *Adapter) Close() error {
	if a == nil || a.state == nil {
		return nil
	}
	a.state.finish(nil)
	<-a.state.done
	return a.state.err
}

func vmnetBufferExhausted(err error) bool {
	var nativeErr *vmnet.Error
	return errors.As(err, &nativeErr) && nativeErr.Domain == "com.apple.vmnet" && nativeErr.Code == 1007
}
