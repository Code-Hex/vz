package datagram

import (
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/Code-Hex/vz/v3/vmnet"
	"github.com/Code-Hex/vz/v3/vmnet/fileadapter"
)

// MARK: - FileAdapterForInterface

// FileAdapterForInterface returns a file for the given [vmnet.Interface].
//   - Invoke the returned function in a separate goroutine to start packet forwarding between the vmnet interface and the file.
//   - The context can be used to stop the goroutines and the interface.
//   - The returned error channel can be used to receive errors from the goroutines.
//   - The connection closure is reported as [io.EOF] error or [syscall.ECONNRESET] error in the error channel.
//
// The returned file can be used as a datagram file descriptor for QEMU, krunkit, or VZ.
//
// QEMU:
//
//	-netdev datagram,id=net0,addr.type=fd,addr.str=<file descriptor>
//	-netdev tap,id=net1,fd=<file descriptor>
//
// krunkit:
//
//	--device virtio-net,type=unixgram,fd=<file descriptor>,offloading=on  // offloading=on is recommended. See krunkit driver in LIMA.
//
// VZ:
//
//	file, errCh, err := FileAdapterForInterface(ctx, iface)
//	attachment := NewFileHandleNetworkDeviceAttachment(file)
var FileAdapterForInterface = fileadapter.ForInterface[*PacketForwarder, net.PacketConn]

// MARK: - PacketForwarder for datagram file adapter

// PacketForwarder implements [fileadapter.PacketForwarder] for datagram file descriptor.
type PacketForwarder struct {
}

var _ fileadapter.PacketForwarder[net.PacketConn] = (*PacketForwarder)(nil)

// New creates a new [PacketForwarder].
func (f *PacketForwarder) New() fileadapter.PacketForwarder[net.PacketConn] {
	return &PacketForwarder{}
}

// Sockopts returns [fileadapter.Sockopts] for the given [vmnet.Interface] and user desired options.
func (*PacketForwarder) Sockopts(iface *vmnet.Interface, userOpts fileadapter.Sockopts) fileadapter.Sockopts {
	return SockoptsForPacketConn(iface, userOpts)
}

// SockoptsForPacketConn returns [fileadapter.Sockopts] for the given [vmnet.Interface] and user desired options for [net.PacketConn].
func SockoptsForPacketConn(iface *vmnet.Interface, userOpts fileadapter.Sockopts) fileadapter.Sockopts {
	// Calculate minimum buffer sizes based on interface configuration
	packetSize := int(iface.MaxPacketSize)
	if iface.EnableVirtioHeader {
		// Add virtio header size
		packetSize += vmnet.VirtioNetHdrSize
	}
	maxPacketCount := max(iface.MaxReadPacketCount, iface.MaxWritePacketCount)
	// On datagram socket, send buffer size only needs to hold one packet.
	minSendBufSize := packetSize
	// Minimum receive buffer size is calculated to hold multiple packets that may handled at once by the vmnet interface.
	defaultRecvBufSize := minSendBufSize * maxPacketCount
	if !iface.EnableTSO {
		// If TSO is disabled, receive buffer size calculated above is too small.
		// Increase receive buffer size to increase performance.
		defaultRecvBufSize *= 10
	}

	// Default socket options
	// When TSO is enabled, both receive buffer sizes will exceed the default maximum buffer size on macOS, it will be capped by the system.
	// default max buffer size on macOS 26.2:
	//  kern.ipc.maxsockbuf: 8388608
	sockopts := fileadapter.Sockopts{
		ReceiveBufferSize: defaultRecvBufSize,
		SendBufferSize:    minSendBufSize,
	}
	if userOpts.ReceiveBufferSize > 0 {
		sockopts.ReceiveBufferSize = defaultRecvBufSize
	}
	if userOpts.SendBufferSize > 0 {
		// If user specified options, override with minimums as needed
		sockopts.SendBufferSize = max(userOpts.SendBufferSize, minSendBufSize)
	}
	return sockopts
}

// ConnAndFile creates a [net.PacketConn] and *[os.File] pair using [syscall.Socketpair].
func (f *PacketForwarder) ConnAndFile(opts fileadapter.Sockopts) (net.PacketConn, *os.File, error) {
	return PacketConnAndFile(opts)
}

// PacketConnAndFile creates a [net.PacketConn] and *[os.File] pair using [syscall.Socketpair].
func PacketConnAndFile(opts fileadapter.Sockopts) (net.PacketConn, *os.File, error) {
	sendBufSize, recvBufSize := opts.SendBufferSize, opts.ReceiveBufferSize
	connFile, file, err := fileadapter.FilePair(syscall.SOCK_DGRAM, sendBufSize, recvBufSize)
	if err != nil {
		return nil, nil, fmt.Errorf("ConnAndFile failed: %w", err)
	}
	conn, err := net.FilePacketConn(connFile)
	if err != nil {
		_ = connFile.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("net.FilePacketConn failed: %w", err)
	}
	if err = connFile.Close(); err != nil {
		_ = conn.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("failed to close connFile: %w", err)
	}
	return conn, file, nil
}

// NewInterfaceToConnForwarder creates a new [fileadapter.InterfaceToConnForwarder] for [net.PacketConn].
func (f *PacketForwarder) NewInterfaceToConnForwarder(iface *vmnet.Interface) fileadapter.InterfaceToConnForwarder[net.PacketConn] {
	maxPacketSize := iface.MaxPacketSize
	if iface.EnableVirtioHeader {
		// Add virtio header size
		maxPacketSize += vmnet.VirtioNetHdrSize
	}
	return &InterfaceToPacketConnForwarder{
		readPktDescsManager: vmnet.NewPktDescsManager(iface.MaxReadPacketCount, maxPacketSize),
	}
}

// NewConnToInterfaceForwarder creates a new [fileadapter.ConnToInterfaceForwarder] for [net.PacketConn].
func (f *PacketForwarder) NewConnToInterfaceForwarder(iface *vmnet.Interface) fileadapter.ConnToInterfaceForwarder[net.PacketConn] {
	maxPacketSize := iface.MaxPacketSize
	if iface.EnableVirtioHeader {
		// Add virtio header size
		maxPacketSize += vmnet.VirtioNetHdrSize
	}
	return &PacketConnToInterfaceForwarder{
		writePktDescsManager: vmnet.NewPktDescsManager(iface.MaxWritePacketCount, maxPacketSize),
	}
}

// MARK: - Interface -> Conn

// InterfaceToPacketConnForwarder forwards packets from [vmnet.Interface] to [net.PacketConn].
type InterfaceToPacketConnForwarder struct {
	readPktDescsManager *vmnet.PktDescsManager
	packetCount         int
}

var _ fileadapter.InterfaceToConnForwarder[net.PacketConn] = (*InterfaceToPacketConnForwarder)(nil)

// ReadPacketsFromInterface reads packets from the [vmnet.Interface].
func (f *InterfaceToPacketConnForwarder) ReadPacketsFromInterface(iface *vmnet.Interface, estimatedCount int) (int, error) {
	f.readPktDescsManager.Reset()
	n, err := iface.ReadPackets(f.readPktDescsManager.PktDescs, estimatedCount)
	f.packetCount = n
	return n, err
}

// WritePacketsToConn writes packets to the [net.PacketConn].
func (f *InterfaceToPacketConnForwarder) WritePacketsToConn(conn net.PacketConn) error {
	return f.readPktDescsManager.WritePacketsToPacketConn(conn, f.packetCount)
}

// MARK: - Conn -> Interface

// PacketConnToInterfaceForwarder forwards packets from [net.PacketConn] to [vmnet.Interface].
type PacketConnToInterfaceForwarder struct {
	writePktDescsManager *vmnet.PktDescsManager
	packetCount          int
}

var _ fileadapter.ConnToInterfaceForwarder[net.PacketConn] = (*PacketConnToInterfaceForwarder)(nil)

// ReadPacketsFromConn reads packets from the [net.PacketConn].
func (f *PacketConnToInterfaceForwarder) ReadPacketsFromConn(conn net.PacketConn) error {
	n, err := f.writePktDescsManager.ReadPacketsFromPacketConn(conn)
	f.packetCount = n
	return err
}

// WritePacketsToInterface writes packets to the [vmnet.Interface].
func (f *PacketConnToInterfaceForwarder) WritePacketsToInterface(iface *vmnet.Interface) error {
	return iface.WritePackets(f.writePktDescsManager.PktDescs, f.packetCount)
}
