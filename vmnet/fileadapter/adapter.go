package fileadapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/Code-Hex/vz/v4/vmnet"
	"golang.org/x/sys/unix"
)

// Transport selects the frame protocol used by the returned file descriptor.
type Transport uint8

const (
	// Stream uses a four-byte big-endian frame length followed by Ethernet bytes.
	Stream Transport = iota

	// Datagram sends one Ethernet frame per UNIX datagram.
	Datagram
)

// Adapter forwards packets until Close, context cancellation, or a forwarding error.
// Copies share one lifetime. It borrows the Interface and never calls Stop.
type Adapter struct {
	state *adapterState
}

type adapterState struct {
	parent context.Context
	cancel context.CancelFunc
	conn   *net.UnixConn
	once   sync.Once
	done   chan struct{}
	err    error
}

// New creates a socket pair and starts forwarding immediately.
// The caller owns the returned file and must close it separately from the Adapter.
// The file may be passed through exec.Cmd.ExtraFiles to QEMU or krunkit.
func New(ctx context.Context, iface *vmnet.Interface, transport Transport) (*Adapter, *os.File, error) {
	if iface == nil || iface.MaxPacketSize() == 0 {
		return nil, nil, fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	if transport != Stream && transport != Datagram {
		return nil, nil, fmt.Errorf("invalid file adapter transport %d", transport)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	conn, file, err := socketPair(transport, iface.MaxPacketSize())
	if err != nil {
		return nil, nil, err
	}

	run, cancel := context.WithCancel(ctx)
	s := &adapterState{
		parent: ctx,
		cancel: cancel,
		conn:   conn,
		done:   make(chan struct{}),
	}

	var pumps sync.WaitGroup
	pumps.Go(func() {
		s.finish(s.forwardToInterface(iface, transport))
	})
	pumps.Go(func() {
		s.finish(s.forwardToPeer(run, iface, transport))
	})
	go func() {
		pumps.Wait()
		close(s.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			s.finish(ctx.Err())
		case <-s.done:
		}
	}()

	return &Adapter{state: s}, file, nil
}

func (s *adapterState) forwardToInterface(iface *vmnet.Interface, transport Transport) error {
	frame := make([]byte, iface.MaxPacketSize())

	for {
		var n int
		var err error
		if transport == Stream {
			n, err = readStreamFrame(s.conn, frame)
		} else {
			n, err = readDatagramFrame(s.conn, frame)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		err = iface.WritePacket(frame[:n])
		if vmnetBufferExhausted(err) {
			continue
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
	frame := make([]byte, iface.MaxPacketSize())

	for {
		n, err := iface.ReadPacket(ctx, frame)
		if err != nil {
			return err
		}

		if transport == Stream {
			err = writeStreamFrame(s.conn, frame[:n])
		} else {
			err = writeDatagramFrame(s.conn, frame[:n])
			if errors.Is(err, unix.ENOBUFS) {
				continue
			}
		}
		if err != nil {
			return err
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
	})
}

// Close stops both forwarding directions and waits for them to finish.
// It does not close the caller's file or stop the borrowed Interface.
func (a *Adapter) Close() error {
	if a == nil || a.state == nil {
		return nil
	}
	a.state.finish(nil)
	return a.Wait()
}

// Wait joins both forwarding directions and returns their first failure.
// Stream peer EOF and an explicit Close return nil if they cause termination first.
// Earlier failures and cancellation remain the result of subsequent calls.
func (a *Adapter) Wait() error {
	if a == nil || a.state == nil {
		return nil
	}
	<-a.state.done
	return a.state.err
}

func vmnetBufferExhausted(err error) bool {
	var nativeErr *vmnet.Error
	return errors.As(err, &nativeErr) && nativeErr.Domain == "com.apple.vmnet" && nativeErr.Code == 1007
}
