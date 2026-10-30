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
	"time"

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

// Option configures a socket adapter.
type Option func(*adapterConfiguration)

type adapterConfiguration struct {
	convertVirtioHeader bool
}

// WithVirtioHeaderConversion connects a raw Virtio vmnet interface to a peer
// that exchanges plain Ethernet frames, such as QEMU stream/socket networking.
// Start iface with vmnet.WithInterfaceVirtioHeader(true).
// The adapter completes partial checksums and removes incoming Virtio headers.
// The peer must send fully checksummed Ethernet frames; the adapter adds
// zeroed headers to them. Unsupported segmentation or RSC checksum metadata
// stops forwarding. Close returns the error.
//
// With TSO enabled, the adapter tracks TCP handshakes and splits complete-checksum
// TCP frames using the peer's MSS and the interface MTU. Start forwarding before
// connections are established. TCP data from vmnet without an observed handshake
// stops the adapter, even when it fits the MTU. The table holds up to 4096 connections;
// entries expire after 30 idle minutes, or two minutes for incomplete handshakes,
// resets, and fully acknowledged closes. A full table stops forwarding.
// Close releases all entries. TCP over IPv4 options or IPv6 extensions, fragments,
// VLAN frames, and nonzero Virtio GSO types are not supported in this mode. Splitting TCP
// frames with authentication, MPTCP, or unknown options is not supported.
// TCP simultaneous open and data carried on SYN packets are not supported.
func WithVirtioHeaderConversion() Option {
	return func(config *adapterConfiguration) {
		config.convertVirtioHeader = true
	}
}

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
	tcp    *tcpSegmentation
}

// New creates a socket pair and starts forwarding immediately.
// The Adapter owns both sockets and borrows iface without stopping it.
// Dup returns a caller-owned socket to pass through exec.Cmd.ExtraFiles.
// Packets pass through unchanged unless WithVirtioHeaderConversion is set.
// Without conversion, a peer of an interface using Virtio headers must support that
// format and the offload features enabled on iface.
func New(ctx context.Context, iface *vmnet.Interface, transport Transport, opts ...Option) (*Adapter, error) {
	if iface == nil || iface.MaxPacketSize() == 0 {
		return nil, fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	if transport != Stream && transport != Datagram {
		return nil, fmt.Errorf("invalid vmnetsock transport %d", transport)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var config adapterConfiguration
	for _, opt := range opts {
		opt(&config)
	}
	if config.convertVirtioHeader {
		if !iface.VirtioHeaderEnabled() {
			return nil, fmt.Errorf("Virtio header conversion requires interface Virtio headers")
		}
		if iface.TSOEnabled() && (iface.MTU() < 68 || iface.MTU() > 65535) {
			return nil, fmt.Errorf("invalid interface MTU for TCP segmentation: %d", iface.MTU())
		}
		if iface.MaxPacketSize() <= virtioHeaderSize {
			return nil, fmt.Errorf("vmnet packet size cannot hold a Virtio header")
		}
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

	if config.convertVirtioHeader && iface.TSOEnabled() {
		s.tcp = newTCPSegmentation(iface.MTU())
	}

	var pumps sync.WaitGroup
	pumps.Go(func() {
		s.finish(s.forwardToInterface(run, iface, transport, config))
	})
	pumps.Go(func() {
		s.finish(s.forwardToPeer(run, iface, transport, config))
	})
	go func() {
		<-run.Done()
		s.finish(run.Err())
		pumps.Wait()
		if s.tcp != nil {
			s.tcp.clear()
		}
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

func (s *adapterState) forwardToInterface(ctx context.Context, iface *vmnet.Interface, transport Transport, config adapterConfiguration) error {
	size := min(iface.MaxPacketSize(), packetBatchBytes)
	if config.convertVirtioHeader {
		size -= virtioHeaderSize
	}
	batchBytes := min(packetBatchBytes, packetBatchLimit*size)
	var readPackets func() ([][]byte, error)
	if transport == Stream {
		reader := newStreamPacketReader(s.conn, size)
		readPackets = reader.readPackets
	} else {
		raw, err := s.conn.SyscallConn()
		if err != nil {
			return err
		}
		frames := make([][]byte, packetBatchLimit)
		buffer := make([]byte, batchBytes)
		readPackets = func() ([][]byte, error) {
			n, err := readDatagramPackets(raw, frames, buffer, size)
			return frames[:n], err
		}
	}

	var nativeFrames [][]byte
	var nativeBuffer []byte
	if config.convertVirtioHeader {
		nativeFrames = make([][]byte, packetBatchLimit)
		nativeBuffer = make([]byte, batchBytes+packetBatchLimit*virtioHeaderSize)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frames, err := readPackets()
		if config.convertVirtioHeader {
			used := 0
			for i, frame := range frames {
				if s.tcp != nil {
					if e := s.tcp.observePeer(frame, time.Now()); e != nil {
						return e
					}
				}
				nativeFrames[i] = nativeBuffer[used : used+virtioHeaderSize+len(frame)]
				// This header may overlap a previous batch's payload.
				clear(nativeFrames[i][:virtioHeaderSize])
				copy(nativeFrames[i][virtioHeaderSize:], frame)
				used += len(nativeFrames[i])
			}
			frames = nativeFrames[:len(frames)]
		}
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

func (s *adapterState) forwardToPeer(ctx context.Context, iface *vmnet.Interface, transport Transport, config adapterConfiguration) error {
	size := iface.MaxPacketSize()
	count := packetBatchSize(size)
	frames := make([][]byte, count)
	for j := range frames {
		frames[j] = make([]byte, size)
	}
	var peerFrames [][]byte
	outputCount := count
	if s.tcp != nil {
		outputCount = packetBatchSize(s.tcp.mtu + ethernetHeaderSize)
	}
	if config.convertVirtioHeader {
		peerFrames = make([][]byte, outputCount)
		if s.tcp != nil {
			for i := range peerFrames {
				peerFrames[i] = make([]byte, s.tcp.mtu+ethernetHeaderSize)
			}
		}
	}
	var headers [][4]byte
	var vectors net.Buffers
	var raw syscall.RawConn
	if transport == Stream {
		headers = make([][4]byte, outputCount)
		vectors = make(net.Buffers, 2*outputCount)
	} else {
		var err error
		raw, err = s.conn.SyscallConn()
		if err != nil {
			return err
		}
	}
	writePackets := func(packets [][]byte) error {
		if transport == Stream {
			return writeStreamFrames(s.conn, packets, headers, vectors)
		}
		return writeDatagramPackets(raw, packets)
	}

	for {
		n, err := iface.ReadPackets(ctx, frames)
		if err != nil {
			return err
		}
		packets := frames[:n]
		if s.tcp != nil {
			if err := s.forwardTCPPackets(packets, peerFrames, writePackets); err != nil {
				return err
			}
			continue
		}
		if config.convertVirtioHeader {
			for i, frame := range packets {
				peerFrames[i], err = virtioEthernetPacket(frame)
				if err != nil {
					return err
				}
			}
			packets = peerFrames[:n]
		}
		if err := writePackets(packets); err != nil {
			return err
		}
	}
}

// forwardTCPPackets copies each borrowed segment before its scratch buffer is reused.
// Flush full batches so a large frame with a small MSS does not grow the batch.
func (s *adapterState) forwardTCPPackets(packets, peerFrames [][]byte, writePackets func([][]byte) error) error {
	used := 0
	flush := func() error {
		if used == 0 {
			return nil
		}
		err := writePackets(peerFrames[:used])
		used = 0
		return err
	}
	emit := func(frame []byte) error {
		peerFrames[used] = peerFrames[used][:len(frame)]
		copy(peerFrames[used], frame)
		used++
		if used == len(peerFrames) {
			return flush()
		}
		return nil
	}
	for _, frame := range packets {
		plain, err := virtioEthernetPacket(frame)
		if err != nil {
			return err
		}
		if err = s.tcp.forwardNative(plain, time.Now(), emit); err != nil {
			return err
		}
	}
	return flush()
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
