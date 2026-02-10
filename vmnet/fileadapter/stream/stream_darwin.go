package stream

import (
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/Code-Hex/vz/v3/vmnet"
	"github.com/Code-Hex/vz/v3/vmnet/fileadapter"
)

// MARK: - FileAdaptorForInterface

// FileAdaptorForInterface returns a file for the given [Network].
//   - Invoke the returned function in a separate goroutine to start packet forwarding between the vmnet interface and the file.
//   - The context can be used to stop the goroutines and the interface.
//   - The returned error channel can be used to receive errors from the goroutines.
//   - The connection closure is reported as [io.EOF] error or [syscall.ECONNRESET] error in the error channel.
//
// The returned file can be used as a file descriptor for QEMU's netdev stream or socket backend.
//
//	-netdev socket,id=net0,fd=<file descriptor>
//	-netdev stream,id=net0,addr.type=fd,addr.str=<file descriptor>
var FileAdaptorForInterface = fileadapter.ForInterface[*PacketForwarder, net.Conn]

// MARK: - PacketForwarder for stream

// PacketForwarder implements PacketForwarder for stream file descriptor.
type PacketForwarder struct {
	receiveBufferSize int
}

var _ fileadapter.PacketForwarder[net.Conn] = (*PacketForwarder)(nil)

// New creates a new PacketForwarder.
func (*PacketForwarder) New() fileadapter.PacketForwarder[net.Conn] {
	return &PacketForwarder{}
}

// Sockopts returns [fileadapter.Sockopts] for the given [vmnet.Interface] and user desired options for [net.Conn].
// Default values are based on the following references:
//   - https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment/maximumtransmissionunit?language=objc
func (*PacketForwarder) Sockopts(iface *vmnet.Interface, userOpts fileadapter.Sockopts) fileadapter.Sockopts {
	// Calculate minimum buffer sizes based on interface configuration
	packetSize := int(iface.MaxPacketSize) + int(vmnet.HeaderSizeForStream)
	if iface.EnableVirtioHeader {
		// Add virtio header size
		packetSize += vmnet.VirtioNetHdrSize
	}
	maxPacketCount := max(iface.MaxReadPacketCount, iface.MaxWritePacketCount)
	// Minimum send buffer size is calculated to hold multiple packets that may handled at once by the vmnet interface.
	defaultSendBufSize := packetSize * maxPacketCount
	if !iface.EnableTSO {
		// If TSO is disabled, send buffer size calculated above is too small.
		// Increase send buffer size to increase performance.
		defaultSendBufSize *= 10
	}
	defaultRecvBufSize := defaultSendBufSize * 4

	// Default socket options
	// When TSO is enabled, both send and receive buffer sizes will exceed the default maximum buffer size on macOS, they will be capped by the system.
	// default max buffer size on macOS 26.2:
	//  kern.ipc.maxsockbuf: 8388608
	sockopts := fileadapter.Sockopts{
		ReceiveBufferSize: defaultRecvBufSize,
		SendBufferSize:    defaultSendBufSize,
	}
	if userOpts.ReceiveBufferSize > 0 {
		sockopts.ReceiveBufferSize = userOpts.ReceiveBufferSize
	}
	if userOpts.SendBufferSize > 0 {
		sockopts.SendBufferSize = userOpts.SendBufferSize
	}
	return sockopts
}

// connAndFile creates a [net.Conn] and *[os.File] pair using [syscall.Socketpair].
func (f *PacketForwarder) ConnAndFile(opts fileadapter.Sockopts) (net.Conn, *os.File, error) {
	sendBufSize, recvBufSize := opts.SendBufferSize, opts.ReceiveBufferSize
	connFile, file, err := fileadapter.FilePair(syscall.SOCK_STREAM, sendBufSize, recvBufSize)
	if err != nil {
		return nil, nil, fmt.Errorf("ConnAndFile failed: %w", err)
	}
	if f.receiveBufferSize, err = syscall.GetsockoptInt(int(connFile.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF); err != nil {
		_ = connFile.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("GetsockoptInt SO_RCVBUF failed: %w", err)
	}
	conn, err := net.FileConn(connFile)
	if err != nil {
		_ = connFile.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("net.FileConn failed: %w", err)
	}
	if err = connFile.Close(); err != nil {
		_ = conn.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("failed to close connFile: %w", err)
	}
	return conn, file, nil
}

// NewInterfaceToConnForwarder creates a new [fileadapter.InterfaceToConnForwarder] for the given [vmnet.Interface].
func (f *PacketForwarder) NewInterfaceToConnForwarder(iface *vmnet.Interface) fileadapter.InterfaceToConnForwarder[net.Conn] {
	maxPacketSize := iface.MaxPacketSize
	if iface.EnableVirtioHeader {
		// Add virtio header size
		maxPacketSize += vmnet.VirtioNetHdrSize
	}
	return &PacketInterfaceToConnForwarder{
		readPktDescsManager: vmnet.NewPktDescsManager(iface.MaxReadPacketCount, maxPacketSize),
		receiveBufferSize:   f.receiveBufferSize,
	}
}

// NewConnToInterfaceForwarder creates a new [fileadapter.ConnToInterfaceForwarder] for the given [vmnet.Interface].
func (f *PacketForwarder) NewConnToInterfaceForwarder(iface *vmnet.Interface) fileadapter.ConnToInterfaceForwarder[net.Conn] {
	maxPacketSize := iface.MaxPacketSize
	if iface.EnableVirtioHeader {
		// Add virtio header size
		maxPacketSize += vmnet.VirtioNetHdrSize
	}
	return &PacketConnToInterfaceForwarder{
		writePktDescsManager: vmnet.NewPktDescsManager(iface.MaxWritePacketCount, maxPacketSize),
		receiveBufferSize:    f.receiveBufferSize,
	}
}

// MARK: - Interface -> Conn

// PacketInterfaceToConnForwarder forwards packets from [vmnet.Interface] to [net.Conn].
type PacketInterfaceToConnForwarder struct {
	readPktDescsManager *vmnet.PktDescsManager
	receiveBufferSize   int
	packetCount         int
}

var _ fileadapter.InterfaceToConnForwarder[net.Conn] = (*PacketInterfaceToConnForwarder)(nil)

// ReadPacketsFromInterface reads packets from the [vmnet.Interface].
func (f *PacketInterfaceToConnForwarder) ReadPacketsFromInterface(iface *vmnet.Interface, estimatedCount int) (int, error) {
	f.readPktDescsManager.Reset()
	n, err := iface.ReadPackets(f.readPktDescsManager.PktDescs, estimatedCount)
	f.packetCount = n
	return n, err
}

// WritePacketsToConn writes packets to the [net.Conn].
func (f *PacketInterfaceToConnForwarder) WritePacketsToConn(conn net.Conn) error {
	return f.readPktDescsManager.WritePacketsToConn(conn, f.packetCount, f.receiveBufferSize)
}

// MARK: - Conn -> Interface

// PacketConnToInterfaceForwarder forwards packets from [net.Conn] to [vmnet.Interface].
type PacketConnToInterfaceForwarder struct {
	writePktDescsManager *vmnet.PktDescsManager
	receiveBufferSize    int
	packetCount          int
}

var _ fileadapter.ConnToInterfaceForwarder[net.Conn] = (*PacketConnToInterfaceForwarder)(nil)

// ReadPacketsFromConn reads packets from the [net.Conn].
func (f *PacketConnToInterfaceForwarder) ReadPacketsFromConn(conn net.Conn) error {
	n, err := f.writePktDescsManager.ReadPacketsFromConn(conn)
	f.packetCount = n
	return err
}

// WritePacketsToInterface writes packets to the [vmnet.Interface].
func (f *PacketConnToInterfaceForwarder) WritePacketsToInterface(iface *vmnet.Interface) error {
	return iface.WritePackets(f.writePktDescsManager.PktDescs, f.packetCount)
}
