package vmnet

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework vmnet
# include "msg_x_darwin.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"iter"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// MARK: - DatagramNextFileAdaptorForInterface

// DatagramNextFileAdaptorForInterface returns a file for the given [Network].
//   - It uses [recvmsg_x] and [sendmsg_x] for packet transfer.
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
//	file, errCh, err := DatagramNextFileAdaptorForInterface(ctx, iface)
//	attachment := NewFileHandleNetworkDeviceAttachment(file)
//
// [recvmsg_x]: https://github.com/apple-oss-distributions/xnu/blob/94d3b452840153a99b38a3a9659680b2a006908e/bsd/sys/socket.h#L1425-L1455
// [sendmsg_x]: https://github.com/apple-oss-distributions/xnu/blob/94d3b452840153a99b38a3a9659680b2a006908e/bsd/sys/socket.h#L1457-L1487
var DatagramNextFileAdaptorForInterface = FileAdaptorForInterface[*DatagramNextPacketForwarder, net.PacketConn]

// MARK: - DatagramNextPacketForwarder for datagram file adaptor

// DatagramNextPacketForwarder implements PacketForwarder for datagram file descriptor by using [recvmsg_x] and [sendmsg_x].
// See: [DatagramNextFileAdaptorForInterface]
type DatagramNextPacketForwarder struct {
	readMsgHdrsArray  *msgHdrXArray
	writeMsgHdrsArray *msgHdrXArray
	localAddr         net.Addr
}

var _ PacketForwarder[net.PacketConn] = (*DatagramNextPacketForwarder)(nil)

// New creates a new DatagramNextPacketForwarder.
func (*DatagramNextPacketForwarder) New() PacketForwarder[net.PacketConn] {
	return &DatagramNextPacketForwarder{}
}

// Sockopts returns socket options for the given Interface and user desired options.
// Default values are based on the following references:
//   - https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment/maximumtransmissionunit?language=objc
func (*DatagramNextPacketForwarder) Sockopts(iface *Interface, userOpts Sockopts) Sockopts {
	// Same as DatagramPacketForwarder
	return sockoptsForPacketConn(iface, userOpts)
}

// ConnAndFile creates a [net.PacketConn] and *[os.File] pair using [syscall.Socketpair].
func (f *DatagramNextPacketForwarder) ConnAndFile(opts Sockopts) (net.PacketConn, *os.File, error) {
	conn, file, err := packetConnAndFile(opts)
	if err != nil {
		return nil, nil, err
	}
	// Save local address for initialize msgHdrX
	f.localAddr = conn.LocalAddr()
	return conn, file, nil
}

// AllocateBuffers allocates message header buffers for reading and writing packets.
func (f *DatagramNextPacketForwarder) AllocateBuffers(iface *Interface) error {
	maxPacketSize := iface.MaxPacketSize
	if iface.EnableVirtioHeader {
		// Add virtio header size
		maxPacketSize += virtioNetHdrSize
	}
	f.readMsgHdrsArray = newMsgHdrXArray(iface.MaxReadPacketCount, maxPacketSize, f.localAddr)
	f.writeMsgHdrsArray = newMsgHdrXArray(iface.MaxWritePacketCount, maxPacketSize, f.localAddr)
	return nil
}

// ReadPacketsFromInterface reads packets from the vmnet Interface.
func (f *DatagramNextPacketForwarder) ReadPacketsFromInterface(iface *Interface, estimatedCount int) (int, error) {
	f.readMsgHdrsArray.reset()
	return iface.ReadPackets(f.readMsgHdrsArray.pktDescsMgr.pktDescs, estimatedCount)
}

// WritePacketsToConn writes packets to the connection.
func (f *DatagramNextPacketForwarder) WritePacketsToConn(conn net.PacketConn, packetCount int) error {
	return f.readMsgHdrsArray.writePacketsToPacketConn(conn, packetCount)
}

// ReadPacketsFromConn reads packets from the connection.
func (f *DatagramNextPacketForwarder) ReadPacketsFromConn(conn net.PacketConn) (int, error) {
	return f.writeMsgHdrsArray.readPacketsFromPacketConn(conn)
}

// WritePacketsToInterface writes packets to the vmnet Interface.
func (f *DatagramNextPacketForwarder) WritePacketsToInterface(iface *Interface, packetCount int) error {
	return iface.WritePackets(f.writeMsgHdrsArray.pktDescsMgr.pktDescs, packetCount)
}

// MARK: - msgHdrXArray and its methods

// msgHdrX is a Go representation of C.struct_msghdr_x.
type msgHdrX C.struct_msghdr_x

// msgHdrXArray manages an array of [msgHdrX] and its [pktDescsManager].
type msgHdrXArray struct {
	msgHdrs     *msgHdrX
	pktDescsMgr *pktDescsManager
}

// newMsgHdrXArray allocates [msgHdrX] array and [pktDescsManager].
// The [msgHdrX]'s iov points to the [VMPktDesc]s' iov.
func newMsgHdrXArray(count int, maxPacketSize uint64, _ net.Addr) *msgHdrXArray {
	m := &msgHdrXArray{
		msgHdrs:     (*msgHdrX)(C.allocateMsgHdrXArray(C.int(count))),
		pktDescsMgr: newPktDescsManager(count, maxPacketSize),
	}
	// sa, len, err := addrToSockaddr(addr)
	// if err != nil {
	// 	panic(fmt.Sprintf("addrToSockaddr failed: %v", err))
	// }
	runtime.AddCleanup(m, func(self *C.struct_msghdr_x) { C.deallocateMsgHdrXArray(self) }, (*C.struct_msghdr_x)(m.msgHdrs))
	// Initialize msgHdrX's iov to point to pktDescs' iov
	for msgHdrX, pktDesc := range m.iter(count) {
		msgHdrX.msg_name = nil
		msgHdrX.msg_namelen = 0
		msgHdrX.msg_iov = pktDesc.vm_pkt_iov
		msgHdrX.msg_iovlen = 1
	}
	return m
}

// at returns the msgHdrX at index i.
func (m *msgHdrXArray) at(i int) *msgHdrX {
	return (*msgHdrX)(unsafe.Pointer(uintptr(unsafe.Pointer(m.msgHdrs)) + uintptr(i)*unsafe.Sizeof(msgHdrX{})))
}

// iter iterates over the msgHdrXArray.
func (m *msgHdrXArray) iter(packetCount int) iter.Seq2[*msgHdrX, *VMPktDesc] {
	return func(yield func(*msgHdrX, *VMPktDesc) bool) {
		for i := range packetCount {
			if !yield(m.at(i), m.pktDescsMgr.at(i)) {
				return
			}
		}
	}
}

// reset resets the pktDescsManager and updates msg_datalen for each msgHdrX.
func (m *msgHdrXArray) reset() {
	m.pktDescsMgr.reset()
	m.clearDataLenAndFlags()
}

// clearDataLenAndFlags updates msg_datalen from msg_iov.iov_len for each msgHdrX.
func (m *msgHdrXArray) clearDataLenAndFlags() {
	for msgHdrX := range m.iter(m.pktDescsMgr.maxPacketCount) {
		msgHdrX.msg_datalen = 0
		msgHdrX.msg_flags = 0
	}
}

// writePacketsToPacketConn writes packets from the [msgHdrX]s to the [net.PacketConn].
//   - It returns an error if any occurs during sending packets.
func (m *msgHdrXArray) writePacketsToPacketConn(conn net.PacketConn, packetCount int) error {
	m.clearDataLenAndFlags()
	// Get rawConn for C.sendmsg_x
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var sentCount int
	var sendErr error
	rawConnWriteErr := rawConn.Write(func(fd uintptr) (done bool) {
		for sentCount < packetCount {
			// send packet from msgHdrX array
			n, err := C.sendmsg_x(C.int(fd), (*C.struct_msghdr_x)(m.at(sentCount)), C.u_int(packetCount-sentCount), 0)
			if n < 0 {
				if errors.Is(err, syscall.EAGAIN) {
					return false // try again later
				} else if errors.Is(err, syscall.ENOBUFS) {
					// Wait and try to send next packet
					time.Sleep(100 * time.Microsecond)
					continue
				}
				sendErr = fmt.Errorf("sendmsg_x failed: %w", err)
				return true
			}
			sentCount += int(n)
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

// readPacketsFromPacketConn reads packets from the [net.PacketConn] into [msgHdrX]s.
//   - It returns the number of packets read.
//   - The packets are read in batch by [recvmsg_x].
//   - It receives all available packets until no more packets are available, packetCount reaches maxPacketCount, or an error occurs.
//   - It waits for the connection to be ready for initial packet.
func (m *msgHdrXArray) readPacketsFromPacketConn(conn net.PacketConn) (int, error) {
	m.reset()
	// Get rawConn for C.recvmsg_x
	var packetCount int
	// Get rawConn for C.recvmsg_x
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var recvErr error
	rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
		// receive packets into msgHdrXs (blocking)
		n, err := C.recvmsg_x(C.int(fd), (*C.struct_msghdr_x)(m.msgHdrs), C.u_int(m.pktDescsMgr.maxPacketCount), 0)
		if n < 0 {
			if errors.Is(err, syscall.EAGAIN) {
				return false // try again later
			}
			recvErr = fmt.Errorf("recvmsg_x failed: %w", err)
			return true
		}
		packetCount = int(n)
		return true
	})
	if rawConnReadErr != nil {
		return 0, fmt.Errorf("rawConn.Read failed: %w", rawConnReadErr)
	}
	if recvErr != nil {
		return 0, recvErr
	}
	for msgHdrX, pktDesc := range m.iter(packetCount) {
		// Update pktDesc's packet size from msg_iov.iov_len
		pktDesc.SetPacketSize(int(msgHdrX.msg_datalen))
	}
	return packetCount, nil
}
