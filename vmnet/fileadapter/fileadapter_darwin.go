package vmnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
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

// PacketForwarder defines methods to forward packets between a vmnet [Interface] and a connection [T io.Closer].
type PacketForwarder[T io.Closer] interface {
	// New creates a new PacketForwarder instance.
	New() PacketForwarder[T]

	// Preparetion

	// Sockopts returns socket options for the given [Interface] and user desired options.
	// Optimal options are calculated based on the Interface configuration and merged with userOpts.
	// If userOpts specify buffer sizes smaller than optimal, optimal sizes are used instead.
	Sockopts(iface *Interface, userOpts Sockopts) Sockopts

	// ConnAndFile creates a connection [T] and its corresponding *[os.File] pair.
	// The connection is used for packet forwarding from/to the [Interface], which actual type is [net.Conn] or [net.PacketConn].
	// The file is used as a file descriptor for QEMU or Virtualization frameworks.
	// The socket options are applied to both ends of the pair.
	ConnAndFile(sockopts Sockopts) (T, *os.File, error)
	// AllocateBuffers allocates packet descriptor buffers based on the [Interface] configuration.
	AllocateBuffers(iface *Interface) error

	// Interface -> Conn

	// ReadPacketsFromInterface reads packets from the vmnet [Interface].
	// It returns the number of packets read.
	ReadPacketsFromInterface(iface *Interface, estimatedCount int) (int, error)
	// WritePacketsToConn writes packets to the connection.
	WritePacketsToConn(conn T, packetCount int) error

	// Conn -> Interface

	// ReadPacketsFromConn reads packets from the connection.
	// It returns the number of packets read.
	ReadPacketsFromConn(conn T) (int, error)
	// WritePacketsToInterface writes packets to the vmnet [Interface].
	WritePacketsToInterface(iface *Interface, packetCount int) error
}

// FileAdaptorForInterface is a generic function that returns a file for the given [Network].
// The returned file is used as a file descriptor for network devices in QEMU, krunkit, or Virtualization frameworks.
//   - Invoke the returned function in a separate goroutine to start packet forwarding between the vmnet interface and the file.
//   - The context can be used to stop the goroutines and the interface.
//   - The returned error channel can be used to receive errors from the goroutines.
//   - The connection closure is reported as [io.EOF] error or [syscall.ECONNRESET] error in the error channel.
func FileAdaptorForInterface[T PacketForwarder[U], U io.Closer](ctx context.Context, iface *Interface, opts ...Sockopt) (file *os.File, start func(), errCh <-chan error, err error) {
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

	// Allocate buffers based on interface configuration
	if err := forwarder.AllocateBuffers(iface); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to allocate buffers: %w", err)
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

		// Set packets available event packetAvailableEventCallback to read packets from vmnet interface
		packetAvailableEventCallback := func(estimatedCount int) {
			for estimatedCount > 0 {
				var packetCount int
				// Read packets from vmnet interface
				if packetCount, err = forwarder.ReadPacketsFromInterface(iface, estimatedCount); err != nil {
					reportError(err, "forwarder.ReadPacketsFromInterface failed")
					return
				}
				// Write packets to the connection
				if err := forwarder.WritePacketsToConn(conn, packetCount); err != nil {
					reportError(err, "forwarder.WritePacketsToConn failed")
					reportError(conn.Close(), "failed to close conn after WritePacketsToConn error")
					return
				}
				estimatedCount -= packetCount
			}
		}
		if err := iface.SetPacketsAvailableEventCallback(packetAvailableEventCallback); err != nil {
			reportError(err, "SetPacketsAvailableEventCallback failed")
			return
		}
		// Start reading packet from the connection (VM) and writing to vmnet interface.
		// Packets comes one by one with 4-byte big-endian header indicating the packet size.
		// Read all available packets in a loop.
		for {
			// Read packets from the connection to writeDescs
			// It won't return until at least one packet is read or connection is closed.
			// Remote closure may be detected as io.EOF on stream connection.
			packetCount, err := forwarder.ReadPacketsFromConn(conn)
			if err != nil {
				reportError(err, "forwarder.ReadPacketsFromConn failed")
				break
			}
			// Write packets to vmnet interface
			if err := forwarder.WritePacketsToInterface(iface, packetCount); err != nil {
				reportError(err, fmt.Sprintf("forwarder.WritePacketsToInterface failed with packetCount=%d", packetCount))
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
