package vz

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/Code-Hex/vz/v4/diskimagekit"
	"github.com/Code-Hex/vz/v4/internal/nbd"
)

// DiskImageServer serves a disk image over the Network Block Device protocol.
// It owns its listener and client connections and borrows the image. It must be
// created with NewDiskImageServer and must not be copied.
//
// The server runs independently of any attachment. Stop every VM using the
// server before calling Close, then close the image separately.
type DiskImageServer struct {
	server    *nbd.Server
	listener  net.Listener
	serverURL string
	dir       string
	served    chan struct{}
	serveErr  error
	once      sync.Once
	closeErr  error
}

type diskImageServerOptions struct {
	listener  net.Listener
	serverURL string
}

// DiskImageServerOption configures NewDiskImageServer.
type DiskImageServerOption func(*diskImageServerOptions) error

// WithDiskImageServerListener serves the image on listener and makes URL return
// serverURL, the connection URL for NBD clients. It must reach listener, directly
// or through a proxy, and use the empty NBD export name. For example:
//
//	nbd://127.0.0.1:10809/
//	nbd+unix:///?socket=/tmp/vz-disk/disk.sock
//
// The server does not validate or connect to this URL. Without this option,
// NewDiskImageServer creates a Unix socket in a private temporary directory.
//
// A successful constructor takes ownership of the selected listener. On failure,
// the listener remains owned by the caller. A listener replaced by a later option
// is never used or closed. The caller must provide any access controls because
// the NBD server has no authentication or encryption.
func WithDiskImageServerListener(listener net.Listener, serverURL string) DiskImageServerOption {
	return func(options *diskImageServerOptions) error {
		if listener == nil {
			return errors.New("vz: nil disk image server listener")
		}
		if serverURL == "" {
			return errors.New("vz: empty disk image server URL")
		}
		options.listener = listener
		options.serverURL = serverURL
		return nil
	}
}

// NewDiskImageServer starts an NBD server backed by image. It supports RAW, ASIF,
// and stacks through diskimagekit. Options are applied in order. The last value
// for each setting wins. Invalid images or options and listener creation failures
// return an error without taking ownership of a caller-supplied listener.
//
// The caller retains ownership of image and must keep it open until Close
// returns. From construction until Close, do not resize the image, change its
// stack, or access it directly. The server synchronizes every write before
// acknowledging it, regardless of the attachment's synchronization mode.
//
// Pass URL to NewNetworkBlockDeviceStorageDeviceAttachment to create an
// attachment. Configure its timeout, read-only mode, and synchronization mode
// through that constructor:
//
//	server, err := vz.NewDiskImageServer(image)
//	if err != nil {
//		return err
//	}
//	defer server.Close()
//	attachment, err := vz.NewNetworkBlockDeviceStorageDeviceAttachment(
//		server.URL(), 5*time.Second, false, vz.DiskSynchronizationModeFull,
//	)
//	if err != nil {
//		return err
//	}
//	// Configure the VM with attachment and wait for it to stop before returning.
//
// Releasing an attachment does not stop the server. Call Close explicitly.
func NewDiskImageServer(image *diskimagekit.DiskImage, opts ...DiskImageServerOption) (*DiskImageServer, error) {
	var options diskImageServerOptions
	for _, option := range opts {
		if option == nil {
			return nil, errors.New("vz: nil disk image server option")
		}
		if err := option(&options); err != nil {
			return nil, err
		}
	}
	server, err := nbd.NewServer(image)
	if err != nil {
		return nil, err
	}
	var dir string
	if options.listener == nil {
		// macOS user temporary paths can exceed the Unix socket path limit.
		dir, err = os.MkdirTemp("/tmp", "vz-disk-")
		if err != nil {
			return nil, err
		}
		socket := filepath.Join(dir, "disk.sock")
		options.listener, err = net.Listen("unix", socket)
		if err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
		serverURL := url.URL{
			Scheme:   "nbd+unix",
			Path:     "/",
			RawQuery: url.Values{"socket": {socket}}.Encode(),
		}
		options.serverURL = serverURL.String()
	}
	s := &DiskImageServer{
		server:    server,
		listener:  options.listener,
		serverURL: options.serverURL,
		dir:       dir,
		served:    make(chan struct{}),
	}
	go func() {
		s.serveErr = server.Serve(s.listener)
		close(s.served)
	}()
	return s, nil
}

// URL returns the NBD connection URL for NewNetworkBlockDeviceStorageDeviceAttachment.
// The default URL uses a Unix socket and the empty NBD export name.
func (s *DiskImageServer) URL() string {
	return s.serverURL
}

// Close stops accepting clients, disconnects them, and waits for image operations
// to finish. It removes any temporary directory created by NewDiskImageServer.
// It does not close the borrowed image or stop a virtual machine.
//
// Close returns an unexpected listener error or a temporary directory removal
// error. Repeated and concurrent calls wait for shutdown and return the same
// result. A closed server cannot be restarted.
func (s *DiskImageServer) Close() error {
	s.once.Do(func() {
		s.server.Close()
		// Close may run before Serve starts. In that case Serve rejects the
		// listener without taking ownership of it.
		s.listener.Close()
		<-s.served
		if !errors.Is(s.serveErr, net.ErrClosed) {
			s.closeErr = s.serveErr
		}
		s.closeErr = errors.Join(s.closeErr, os.RemoveAll(s.dir))
	})
	return s.closeErr
}
