package vmnet

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework vmnet
# include "vmnet_darwin.h"
*/
import "C"
import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// MARK: - StreamFileAdaptorForInterface

// StreamFileAdaptorForInterface returns a file for the given [Network].
//   - Invoke the returned function in a separate goroutine to start packet forwarding between the vmnet interface and the file.
//   - The context can be used to stop the goroutines and the interface.
//   - The returned error channel can be used to receive errors from the goroutines.
//   - The connection closure is reported as [io.EOF] error or [syscall.ECONNRESET] error in the error channel.
//
// The returned file can be used as a file descriptor for QEMU's netdev stream or socket backend.
//
//	-netdev socket,id=net0,fd=<file descriptor>
//	-netdev stream,id=net0,addr.type=fd,addr.str=<file descriptor>
var StreamFileAdaptorForInterface = FileAdaptorForInterface[*StreamPacketForwarder, net.Conn]

// MARK: - StreamPacketForwarder for stream

// StreamPacketForwarder implements PacketForwarder for stream file descriptor.
type StreamPacketForwarder struct {
	readPktDescsManager  *pktDescsManager
	writePktDescsManager *pktDescsManager
}

var _ PacketForwarder[net.Conn] = (*StreamPacketForwarder)(nil)

// New creates a new StreamPacketForwarder.
func (*StreamPacketForwarder) New() PacketForwarder[net.Conn] {
	return &StreamPacketForwarder{}
}

// Sockopts returns socket options for the given Interface and user desired options.
// Default values are based on the following references:
//   - https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment/maximumtransmissionunit?language=objc
func (*StreamPacketForwarder) Sockopts(iface *Interface, userOpts Sockopts) Sockopts {
	// Calculate minimum buffer sizes based on interface configuration
	packetSize := int(iface.MaxPacketSize) + int(headerSize)
	if iface.EnableVirtioHeader {
		// Add virtio header size
		packetSize += virtioNetHdrSize
	}
	minPacketCount := max(iface.MaxReadPacketCount, iface.MaxWritePacketCount)
	minSendBufSize := packetSize * minPacketCount
	minRecvBufSize := minSendBufSize

	// Default socket options
	sockopts := Sockopts{
		ReceiveBufferSize: minRecvBufSize * 4 * 10,
		SendBufferSize:    minSendBufSize * 1 * 10,
	}
	// If user specified options, override with minimums as needed
	if userOpts.ReceiveBufferSize > 0 {
		sockopts.ReceiveBufferSize = max(userOpts.ReceiveBufferSize, minRecvBufSize)
	}
	if userOpts.SendBufferSize > 0 {
		sockopts.SendBufferSize = max(userOpts.SendBufferSize, minSendBufSize)
	}
	return sockopts
}

// connAndFile creates a [net.Conn] and *[os.File] pair using [syscall.Socketpair].
func (*StreamPacketForwarder) ConnAndFile(opts Sockopts) (net.Conn, *os.File, error) {
	sendBufSize, recvBufSize := opts.SendBufferSize, opts.ReceiveBufferSize
	connFile, file, err := filePair(syscall.SOCK_STREAM, sendBufSize, recvBufSize)
	if err != nil {
		return nil, nil, fmt.Errorf("ConnAndFile failed: %w", err)
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

// AllocateBuffers allocates packet descriptor buffers for reading and writing packets.
func (f *StreamPacketForwarder) AllocateBuffers(iface *Interface) error {
	maxPacketSize := iface.MaxPacketSize
	if iface.EnableVirtioHeader {
		// Add virtio header size
		maxPacketSize += virtioNetHdrSize
	}
	f.readPktDescsManager = newPktDescsManager(iface.MaxReadPacketCount, maxPacketSize)
	f.writePktDescsManager = newPktDescsManager(iface.MaxWritePacketCount, maxPacketSize)
	return nil
}

// ReadPacketsFromInterface reads packets from the vmnet Interface.
func (f *StreamPacketForwarder) ReadPacketsFromInterface(iface *Interface, estimatedCount int) (int, error) {
	f.readPktDescsManager.reset()
	return iface.ReadPackets(f.readPktDescsManager.pktDescs, estimatedCount)
}

// WritePacketsToConn writes packets to the connection.
func (f *StreamPacketForwarder) WritePacketsToConn(conn net.Conn, packetCount int) error {
	return f.readPktDescsManager.writePacketsToConn(conn, packetCount)
}

// ReadPacketsFromConn reads packets from the connection.
func (f *StreamPacketForwarder) ReadPacketsFromConn(conn net.Conn) (int, error) {
	return f.writePktDescsManager.readPacketsFromConn(conn)
}

// WritePacketsToInterface writes packets to the vmnet Interface.
func (f *StreamPacketForwarder) WritePacketsToInterface(iface *Interface, packetCount int) error {
	return iface.WritePackets(f.writePktDescsManager.pktDescs, packetCount)
}

// MARK: - pktDescsManager methods for stream file adaptor

// buffersForWritingToConn returns [net.Buffers] to write to the [net.Conn]
// adjusted their buffer sizes based vm_pkt_size in [VMPktDesc]s read from [Interface].
func (v *pktDescsManager) buffersForWritingToConn(packetCount int) (net.Buffers, error) {
	for i, vmPktDesc := range v.iter(packetCount) {
		if uint64(vmPktDesc.vm_pkt_size) > v.maxPacketSize {
			return nil, fmt.Errorf("vm_pkt_size %d exceeds maxPacketSize %d", vmPktDesc.vm_pkt_size, v.maxPacketSize)
		}
		// Write packet size to the 4-byte header
		binary.BigEndian.PutUint32(v.backingBuffers[i][:headerSize], uint32(vmPktDesc.vm_pkt_size))
		// Resize buffer to include header and packet size
		v.writingBuffers[i] = v.backingBuffers[i][:headerSize+uintptr(vmPktDesc.vm_pkt_size)]
	}
	return v.writingBuffers[:packetCount], nil
}

// writePacketsToConn writes packets from [VMPktDesc]s to the [net.Conn].
//   - It returns the number of bytes written.
func (v *pktDescsManager) writePacketsToConn(conn net.Conn, packetCount int) error {
	// To use built-in Writev implementation in net package (internal/poll.FD.Writev),
	// we use net.Buffers and its WriteTo method.
	buffers, err := v.buffersForWritingToConn(packetCount)
	if err != nil {
		return fmt.Errorf("buffersForWritingToConn failed: %w", err)
	}
	// Write packets to the connection
	// [Buffers.WriteTo] uses writev syscall internally, it also handles partial writes until all data is written.
	// So, we don't need to handle partial writes here.
	_, err = buffers.WriteTo(conn)
	if err != nil {
		return fmt.Errorf("buffers.WriteTo failed: %w", err)
	}
	return nil
}

// readPacketsFromConn reads packets from the [net.Conn] into [VMPktDesc]s.
//   - It returns the number of packets read.
//   - The packets are expected to come one by one with 4-byte big-endian header indicating the packet size.
//   - It reads all available packets until no more packets are available, packetCount reaches maxPacketCount, or an error occurs.
//   - It waits for the connection to be ready for initial read of 4-byte header.
func (v *pktDescsManager) readPacketsFromConn(conn net.Conn) (int, error) {
	var packetCount int
	// Wait until 4-byte header is read
	if _, err := conn.Read(v.backingBuffers[packetCount][:headerSize]); err != nil {
		return 0, fmt.Errorf("conn.Read failed: %w", err)
	}
	// Get rawConn for Readv
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	// Read available packets
	var packetLen uint32
	var bufs net.Buffers
	for {
		packetLen = binary.BigEndian.Uint32(v.backingBuffers[packetCount][:headerSize])
		if packetLen == 0 || uint64(packetLen) > v.maxPacketSize {
			return 0, fmt.Errorf("invalid packetLen: %d (max %d)", packetLen, v.maxPacketSize)
		}

		// prepare buffers for reading packet and next header if any
		if packetCount+1 < v.maxPacketCount {
			// prepare next header read as well
			bufs = net.Buffers{
				v.backingBuffers[packetCount][headerSize : headerSize+uintptr(packetLen)],
				v.backingBuffers[packetCount+1][:headerSize],
			}
		} else {
			// prepare only packet read to avoid exceeding maxPacketCount
			bufs = net.Buffers{
				v.backingBuffers[packetCount][headerSize : headerSize+uintptr(packetLen)],
			}
		}

		// Read packet from the connection
		var bytesHasBeenRead int
		var err error
		rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
			// read packet into buffers
			bytesHasBeenRead, err = unix.Readv(int(fd), bufs)
			if bytesHasBeenRead <= 0 {
				if errors.Is(err, syscall.EAGAIN) {
					return false // try again later
				}
				err = fmt.Errorf("unix.Readv failed: %w", err)
				return true
			}
			// assumes partial read of a packet does not happen since packet len is already known
			return true
		})
		if rawConnReadErr != nil {
			return 0, fmt.Errorf("rawConn.Read failed: %w", rawConnReadErr)
		}
		if err != nil {
			return 0, fmt.Errorf("closure in rawConn.Read failed: %w", err)
		}
		v.at(packetCount).SetPacketSize(int(packetLen))
		packetCount++
		if bytesHasBeenRead == int(packetLen) {
			// next packet seems not available now, or reached maxPacketCount
			break
		} else if bytesHasBeenRead != int(packetLen)+int(headerSize) {
			return 0, fmt.Errorf("unexpected bytesHasBeenRead: %d (expected %d or %d)", bytesHasBeenRead, packetLen, packetLen+uint32(headerSize))
		}
	}
	return packetCount, nil
}
