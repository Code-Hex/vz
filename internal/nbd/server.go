// Package nbd exports a diskimagekit image or stack over the Network Block
// Device protocol. It supports fixed newstyle negotiation and simple replies.
// The caller can connect the export to a Virtualization.framework network block
// device attachment without a Swift bridge.
//
// The protocol has no authentication or encryption in this package. Use a Unix
// socket in a private directory, or supply a listener with suitable access
// controls. See https://github.com/NetworkBlockDevice/nbd/blob/master/doc/proto.md.
package nbd

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

// Option configures an NBD export.
type Option func(*options)

type options struct {
	readOnly bool
}

// WithReadOnly prevents clients from writing, even when the image is writable.
// An image opened read-only is always exported read-only.
func WithReadOnly() Option {
	return func(options *options) {
		options.readOnly = true
	}
}

// Server exports one image under the empty export name. A Server is single-use
// and must be created with NewServer. Close is safe to call concurrently with Serve.
// It does not close the image.
type Server struct {
	image    *diskimagekit.DiskImage
	size     uint64
	block    uint32
	readOnly bool

	mu       sync.Mutex
	started  bool
	closed   bool
	listener net.Listener
	conns    map[net.Conn]struct{}
	done     chan struct{}
	workers  sync.WaitGroup
}

// NewServer prepares an export. The caller retains ownership of image and must keep
// it open until Server.Close returns. Do not resize the image, change its stack,
// or access it directly while serving clients. Its size and block size are
// fixed for the lifetime of the export.
//
// Writes are synchronized before their success replies, including writes
// without FUA. This also supports clients that present a write-through device
// to their guest. TRIM, write zeroes, and structured replies are not advertised.
func NewServer(image *diskimagekit.DiskImage, opts ...Option) (*Server, error) {
	if image == nil {
		return nil, errors.New("nbd: nil image")
	}
	if _, err := image.ReadAt(nil, 0); err != nil {
		return nil, fmt.Errorf("nbd: image: %w", err)
	}
	size, block := image.Size(), image.BlockSize()
	if size <= 0 || (block != diskimagekit.BlockSize512 && block != diskimagekit.BlockSize4096) || size%int64(block) != 0 {
		return nil, errors.New("nbd: invalid image geometry")
	}
	settings := options{
		readOnly: image.OpenMode() == diskimagekit.ReadOnly,
	}
	for _, option := range opts {
		if option == nil {
			return nil, errors.New("nbd: nil option")
		}
		option(&settings)
	}
	server := &Server{
		image:    image,
		size:     uint64(size),
		block:    uint32(block),
		readOnly: settings.readOnly,
		conns:    make(map[net.Conn]struct{}),
		done:     make(chan struct{}),
	}
	return server, nil
}

// Serve accepts clients on listener until Close is called or Accept fails.
// It owns listener after a successful start and closes it and all accepted
// connections before returning. Each connection processes requests in order.
// Client protocol and I/O errors end that connection; they do not stop Serve.
// Serve returns net.ErrClosed after Close, and otherwise the Accept error.
// Calling Serve more than once is an error and leaves the new listener open.
func (s *Server) Serve(listener net.Listener) error {
	if listener == nil {
		return errors.New("nbd: nil listener")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return net.ErrClosed
	}
	if s.started {
		s.mu.Unlock()
		return errors.New("nbd: Serve already called")
	}
	s.started = true
	s.listener = listener
	s.mu.Unlock()
	defer func() {
		s.shutdown()
		// Only this accept loop adds workers. Once it exits, Wait cannot race
		// with an Add for a newly accepted connection.
		s.workers.Wait()
		close(s.done)
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return net.ErrClosed
			}
			return err
		}
		// Close must either see this connection in conns or prevent its
		// worker from starting. Keep the closed check and registration together.
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			return net.ErrClosed
		}
		s.conns[conn] = struct{}{}
		s.workers.Add(1)
		s.mu.Unlock()
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer s.workers.Done()
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()
	if err := s.negotiate(conn); err == nil {
		s.transmit(conn)
	}
}

// Close stops accepting clients, closes their connections, and waits for
// in-flight image operations to return. It does not interrupt a disk operation
// that is already running. Normally, stop the VM before closing its export.
// Repeated calls wait for the same shutdown and return nil.
func (s *Server) Close() error {
	s.shutdown()
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if started {
		<-s.done
	}
	return nil
}

func (s *Server) shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	listener := s.listener
	connections := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	// Closing transports unblocks network I/O. Keep mu free so workers can
	// remove themselves from conns while shutdown is in progress.
	if listener != nil {
		listener.Close()
	}
	for _, conn := range connections {
		conn.Close()
	}
}
