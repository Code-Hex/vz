package fileadapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/Code-Hex/vz/v3/vmnet"
)

// Sockopts holds socket options for the connection.
type Sockopts struct {
	ReceiveBufferSize int // [syscall.SO_RCVBUF]
	SendBufferSize    int // [syscall.SO_SNDBUF]
}

// Sockopt defines a function type to set socket options.
type Sockopt func(*Sockopts)

// WithReceiveBufferSize sets the receive buffer size (SO_RCVBUF) for the socket.
// Specified size may be overridden to meet minimum requirements.
func WithReceiveBufferSize(size int) Sockopt {
	return func(o *Sockopts) {
		o.ReceiveBufferSize = size
	}
}

// WithSendBufferSize sets the send buffer size (SO_SNDBUF) for the socket.
// Specified size may be overridden to meet minimum requirements.
func WithSendBufferSize(size int) Sockopt {
	return func(o *Sockopts) {
		o.SendBufferSize = size
	}
}

// InterfaceToConnForwarder defines methods to forward packets from a vmnet [vmnet.Interface] to a connection [T io.Closer].
type InterfaceToConnForwarder[T io.Closer] interface {
	// ReadPacketsFromInterface reads packets from the vmnet [vmnet.Interface].
	// It returns the number of packets read.
	ReadPacketsFromInterface(iface *vmnet.Interface, estimatedCount int) (int, error)
	// WritePacketsToConn writes packets to the connection.
	WritePacketsToConn(conn T) error
}

// ConnToInterfaceForwarder defines methods to forward packets from a connection [T io.Closer] to a vmnet [vmnet.Interface].
type ConnToInterfaceForwarder[T io.Closer] interface {
	// ReadPacketsFromConn reads packets from the connection.
	ReadPacketsFromConn(conn T) error
	// WritePacketsToInterface writes packets to the vmnet [vmnet.Interface].
	WritePacketsToInterface(iface *vmnet.Interface) error
}

// PacketForwarder defines methods to forward packets between a vmnet [vmnet.Interface] and a connection [T io.Closer].
type PacketForwarder[T io.Closer] interface {
	// New creates a new PacketForwarder instance.
	New() PacketForwarder[T]

	// Preparetion

	// Sockopts returns socket options for the given [vmnet.Interface] and user desired options.
	// Optimal options are calculated based on the Interface configuration and merged with userOpts.
	// If userOpts specify buffer sizes smaller than optimal, optimal sizes are used instead.
	Sockopts(iface *vmnet.Interface, userOpts Sockopts) Sockopts

	// ConnAndFile creates a connection [T] and its corresponding *[os.File] pair.
	// The connection is used for packet forwarding from/to the [vmnet.Interface], which actual type is [net.Conn] or [net.PacketConn].
	// The file is used as a file descriptor for QEMU or Virtualization frameworks.
	// The socket options are applied to both ends of the pair.
	ConnAndFile(sockopts Sockopts) (T, *os.File, error)

	// Interface -> Conn
	NewInterfaceToConnForwarder(iface *vmnet.Interface) InterfaceToConnForwarder[T]

	// Conn -> Interface
	NewConnToInterfaceForwarder(iface *vmnet.Interface) ConnToInterfaceForwarder[T]
}

// ForInterface is a generic function that returns a file for the given [Network].
// The returned file is used as a file descriptor for network devices in QEMU, krunkit, or Virtualization frameworks.
//   - Invoke the returned function in a separate goroutine to start packet forwarding between the vmnet interface and the file.
//   - The context can be used to stop the goroutines and the interface.
//   - The returned error channel can be used to receive errors from the goroutines.
//   - The connection closure is reported as [io.EOF] error or [syscall.ECONNRESET] error in the error channel.
func ForInterface[T PacketForwarder[U], U io.Closer](ctx context.Context, iface *vmnet.Interface, opts ...Sockopt) (file *os.File, start func(), errCh <-chan error, err error) {
	var factory T
	forwarder := factory.New()

	var userSockopts Sockopts
	for _, opt := range opts {
		opt(&userSockopts)
	}

	// Get socket options from the forwarder
	sockopts := forwarder.Sockopts(iface, userSockopts)

	// Create socketpair connection as conn and file
	conn, file, err := forwarder.ConnAndFile(sockopts)
	if err != nil {
		if err2 := iface.Stop(); err2 != nil {
			return nil, nil, nil, errors.Join(err, fmt.Errorf("failed to stop iface: %w", err2))
		}
		return nil, nil, nil, fmt.Errorf("forwarder.ConnAndFile failed: %w", err)
	}

	// Channel to report errors from goroutine
	errChRW := make(chan error, 10)
	reportError := func(err error, message string) {
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				// Silently ignore the error caused by connection closure
				return
			}
			errChRW <- fmt.Errorf("%s: %w", message, err)
		}
	}
	evaluateAndReportError := func(f func() error, message string) {
		reportError(f(), message)
	}

	start = sync.OnceFunc(func() {
		defer evaluateAndReportError(iface.Stop, "failed to stop iface")
		defer evaluateAndReportError(conn.Close, "failed to close conn")

		// prepare reusing functions for Interface -> Conn
		readingFromInterface, writingToConn := reusingFuncs(ctx,
			func() InterfaceToConnForwarder[U] {
				return forwarder.NewInterfaceToConnForwarder(iface)
			}, 2)

		// Set packets available event packetAvailableEventCallback to read packets from vmnet interface
		packetAvailableEventCallback := func(estimatedCount int) {
			for estimatedCount > 0 {
				if err := readingFromInterface(func(forwarder InterfaceToConnForwarder[U]) error {
					n, err := forwarder.ReadPacketsFromInterface(iface, estimatedCount)
					if err != nil {
						return err
					}
					estimatedCount -= n
					return nil
				}); err != nil {
					reportError(err, "ReadPacketsFromInterface failed")
					return
				}
			}
		}
		if err := iface.SetPacketsAvailableEventCallback(packetAvailableEventCallback); err != nil {
			reportError(err, "SetPacketsAvailableEventCallback failed")
			return
		}

		// Start goroutine to write packets from the vmnet interface to the connection.
		go func() {
			for {
				if err := writingToConn(func(forwarder InterfaceToConnForwarder[U]) error {
					return forwarder.WritePacketsToConn(conn)
				}); err != nil {
					reportError(err, "WritePacketsToConn failed")
					reportError(conn.Close(), "failed to close conn after WritePacketsToConn error")
					return
				}
			}
		}()

		// prepare reusing functions for Conn -> Interface
		readingFromConn, writingToInterface := reusingFuncs(ctx,
			func() ConnToInterfaceForwarder[U] {
				return forwarder.NewConnToInterfaceForwarder(iface)
			}, 2)
		// Start goroutine to write packets from the connection to the vmnet interface.
		go func() {
			for {
				if err := writingToInterface(func(forwarder ConnToInterfaceForwarder[U]) error {
					return forwarder.WritePacketsToInterface(iface)
				}); err != nil {
					reportError(err, "WritePacketsToInterface failed")
					reportError(conn.Close(), "failed to close conn after WritePacketsToInterface error")
					return
				}
			}
		}()
		// Start reading packet from the connection (VM) and writing to the vmnet interface.
		for {
			if err := readingFromConn(func(forwarder ConnToInterfaceForwarder[U]) error {
				return forwarder.ReadPacketsFromConn(conn)
			}); err != nil {
				reportError(err, "ReadPacketsFromConn failed")
				reportError(conn.Close(), "failed to close conn after ReadPacketsFromConn error")
				break
			}
		}
		// Keep readBuffers and writeBuffers alive until the goroutine ends
		runtime.KeepAlive(forwarder)
		runtime.KeepAlive(iface)
	})

	go func() {
		<-ctx.Done()
		if err := conn.Close(); err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				reportError(err, "failed to close conn on context done")
			}
		}
	}()

	return file, start, errChRW, nil
}

// reusingFuncs returns a pair of functions for reading and writing with reusing objects.
// The reading function reuses objects passed to the writing function via channels.
// If no reusable object is available, a new object is created using the factory function.
// The queueLen parameter specifies the length of the channels used for reusing objects.
// The context can be used to stop the functions.
func reusingFuncs[T any](ctx context.Context, factory func() T, queueLen int) (func(func(T) error) error, func(func(T) error) error) {
	forwardingCh := make(chan T, queueLen)
	reusingCh := make(chan T, queueLen)
	// readingFunc reads an item from reusingCh or creates a new one using factory, then applies f to it, and sends it to forwardingCh.
	readingFunc := func(f func(T) error) error {
		var item T
		var ok bool
		select {
		case item, ok = <-reusingCh:
			if !ok {
				return fmt.Errorf("readingFunc: reusingCh closed")
			}
		case <-ctx.Done():
			close(forwardingCh)
			return fmt.Errorf("readingFunc: context done")
		default:
			item = factory()
		}
		if err := f(item); err != nil {
			return err
		}
		forwardingCh <- item
		return nil
	}
	// writingFunc reads an item from forwardingCh, applies f to it, and sends it to reusingCh.
	// If reusingCh is full, the item is dropped.
	writingFunc := func(f func(T) error) error {
		if item, ok := <-forwardingCh; !ok {
			close(reusingCh)
			return fmt.Errorf("writingFunc: forwardingCh closed")
		} else if err := f(item); err != nil {
			return err
		} else {
			select {
			case reusingCh <- item:
			default:
				// drop the item if the channel is full
			}
		}
		return nil
	}
	return readingFunc, writingFunc
}
