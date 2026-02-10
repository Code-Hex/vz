package vmnet

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework vmnet
# include "vmnet_darwin.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// MARK: - DatagramFileAdaptorForInterface

// DatagramFileAdaptorForInterface returns a file for the given [Network].
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
//	file, errCh, err := DatagramFileAdaptorForInterface(ctx, iface)
//	attachment := NewFileHandleNetworkDeviceAttachment(file)
var DatagramFileAdaptorForInterface = FileAdaptorForInterface[*DatagramPacketForwarder, net.PacketConn]

// MARK: - DatagramPacketForwarder for datagram file adaptor

// DatagramPacketForwarder implements PacketForwarder for datagram file descriptor.
type DatagramPacketForwarder struct {
	readPktDescsManager  *pktDescsManager
	writePktDescsManager *pktDescsManager
}

var _ PacketForwarder[net.PacketConn] = (*DatagramPacketForwarder)(nil)

// New creates a new DatagramPacketForwarder.
func (f *DatagramPacketForwarder) New() PacketForwarder[net.PacketConn] {
	return &DatagramPacketForwarder{}
}

// Sockopts returns socket options for the given Interface and user desired options.
// Default values are based on the following references:
//   - https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment/maximumtransmissionunit?language=objc
func (*DatagramPacketForwarder) Sockopts(iface *Interface, userOpts Sockopts) Sockopts {
	return sockoptsForPacketConn(iface, userOpts)
}

// ConnAndFile creates a [net.PacketConn] and *[os.File] pair using [syscall.Socketpair].
func (f *DatagramPacketForwarder) ConnAndFile(opts Sockopts) (net.PacketConn, *os.File, error) {
	return packetConnAndFile(opts)
}

// AllocateBuffers allocates packet descriptor buffers for reading and writing packets.
func (f *DatagramPacketForwarder) AllocateBuffers(iface *Interface) error {
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
func (f *DatagramPacketForwarder) ReadPacketsFromInterface(iface *Interface, estimatedCount int) (int, error) {
	f.readPktDescsManager.reset()
	return iface.ReadPackets(f.readPktDescsManager.pktDescs, estimatedCount)
}

// WritePacketsToConn writes packets to the connection.
func (f *DatagramPacketForwarder) WritePacketsToConn(conn net.PacketConn, packetCount int) error {
	return f.readPktDescsManager.writePacketsToPacketConn(conn, packetCount)
}

// ReadPacketsFromConn reads packets from the connection.
func (f *DatagramPacketForwarder) ReadPacketsFromConn(conn net.PacketConn) (int, error) {
	return f.writePktDescsManager.readPacketsFromPacketConn(conn)
}

// WritePacketsToInterface writes packets to the vmnet Interface.
func (f *DatagramPacketForwarder) WritePacketsToInterface(iface *Interface, packetCount int) error {
	return iface.WritePackets(f.writePktDescsManager.pktDescs, packetCount)
}

// sockoptsForPacketConn returns socket options for the given [Interface] and user desired options for [net.PacketConn].
// Default values are based on the following references:
//   - https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment/maximumtransmissionunit?language=objc
func sockoptsForPacketConn(iface *Interface, userOpts Sockopts) Sockopts {
	// Calculate minimum buffer sizes based on interface configuration
	packetSize := int(iface.MaxPacketSize)
	if iface.EnableVirtioHeader {
		// Add virtio header size
		packetSize += virtioNetHdrSize
	}
	minPacketCount := max(iface.MaxReadPacketCount, iface.MaxWritePacketCount)
	minSendBufSize := packetSize
	minRecvBufSize := minSendBufSize * minPacketCount

	// Default socket options
	sockopts := Sockopts{
		ReceiveBufferSize: minRecvBufSize * 4 * 10,
		SendBufferSize:    packetSize,
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

// packetConnAndFile creates a [net.PacketConn] and *[os.File] pair using [syscall.Socketpair].
func packetConnAndFile(opts Sockopts) (net.PacketConn, *os.File, error) {
	sendBufSize, recvBufSize := opts.SendBufferSize, opts.ReceiveBufferSize
	connFile, file, err := filePair(syscall.SOCK_DGRAM, sendBufSize, recvBufSize)
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

// MARK: - pktDescsManager methods for datagram file adaptor

// buffersForWritingToPacketConn returns [net.Buffers] to write to the [net.PacketConn]
// adjusted their buffer sizes based vm_pkt_size in [VMPktDesc]s read from [Interface].
// The 4-byte header is excluded.
func (v *pktDescsManager) buffersForWritingToPacketConn(packetCount int) (net.Buffers, error) {
	for i, vmPktDesc := range v.iter(packetCount) {
		if uint64(vmPktDesc.vm_pkt_size) > v.maxPacketSize {
			return nil, fmt.Errorf("vm_pkt_size %d exceeds maxPacketSize %d", vmPktDesc.vm_pkt_size, v.maxPacketSize)
		}
		// Resize buffer to exclude the 4-byte header
		v.writingBuffers[i] = v.packetBufferAt(i, 0)
	}
	return v.writingBuffers[:packetCount], nil
}

// writePacketsToPacketConn writes packets from [VMPktDesc]s to the [net.PacketConn].
//   - It returns an error if any occurs during sending packets.
func (v *pktDescsManager) writePacketsToPacketConn(conn net.PacketConn, packetCount int) error {
	buffers, err := v.buffersForWritingToPacketConn(packetCount)
	if err != nil {
		return fmt.Errorf("buffersForWritingToPacketConn failed: %w", err)
	}
	// Get rawConn for syscall.Sendmsg
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var sentCount int
	var sendErr error
	rawConnWriteErr := rawConn.Write(func(fd uintptr) (done bool) {
		for sentCount < packetCount {
			// send packet from buffer
			if err := syscall.Sendmsg(int(fd), buffers[sentCount], nil, nil, 0); err != nil {
				if errors.Is(err, syscall.EAGAIN) {
					return false // try again later
				} else if errors.Is(err, syscall.ENOBUFS) {
					// Wait and try to send next packet
					time.Sleep(100 * time.Microsecond)
					continue
				}
				sendErr = fmt.Errorf("syscall.Sendmsg failed: %w", err)
				return true
			}
			sentCount++
		}
		return true
	})
	if rawConnWriteErr != nil {
		return fmt.Errorf("rawConn.Write failed: %w", rawConnWriteErr)
	}
	if sendErr != nil {
		return sendErr
	}
	return nil
}

// readPacketsFromPacketConn reads packets from the [net.PacketConn] into [VMPktDesc]s.
//   - It returns the number of packets read.
//   - The packets are expected to come one by one.
//   - It receives all available packets until no more packets are available, packetCount reaches maxPacketCount, or an error occurs.
//   - It waits for the connection to be ready for initial packet.
func (v *pktDescsManager) readPacketsFromPacketConn(conn net.PacketConn) (int, error) {
	var packetCount int
	// Read the first packet (blocking)
	n, _, err := conn.ReadFrom(v.backingBuffers[packetCount][headerSize:])
	if n == 0 {
		// normal closure. Will this happen in datagram socket?
		return 0, errors.New("conn.ReadFrom: use of closed network connection")
	}
	if err != nil {
		return 0, fmt.Errorf("conn.ReadFrom failed: %w", err)
	}
	v.at(packetCount).SetPacketSize(n)
	packetCount++
	// Get rawConn for syscall.Recvfrom
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var recvErr error
	rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
		// Read available packets until no more packets are available or packetCount reaches maxPacketCount
		for packetCount < v.maxPacketCount {
			// receive packet into buffer
			n, _, err := syscall.Recvfrom(int(fd), v.backingBuffers[packetCount][headerSize:], 0)
			if err != nil {
				if !errors.Is(err, syscall.EAGAIN) {
					recvErr = fmt.Errorf("syscall.Recvfrom failed: %w", err)
				}
				return true // Do not retry on error
			}
			v.at(packetCount).SetPacketSize(n)
			packetCount++
		}
		return true
	})
	if rawConnReadErr != nil {
		return 0, fmt.Errorf("rawConn.Read failed: %w", rawConnReadErr)
	}
	if recvErr != nil {
		return 0, recvErr
	}
	return packetCount, nil
}
