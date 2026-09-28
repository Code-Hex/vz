package datagramx

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
	"unsafe"

	"github.com/Code-Hex/vz/v3/vmnet"
	"github.com/Code-Hex/vz/v3/vmnet/fileadapter"
	"github.com/Code-Hex/vz/v3/vmnet/fileadapter/datagram"
)

// MARK: - FileAdapterForInterface

// FileAdapterForInterface returns a file for the given [Network].
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
//	file, errCh, err := FileAdapterForInterface(ctx, iface)
//	attachment := NewFileHandleNetworkDeviceAttachment(file)
//
// [recvmsg_x]: https://github.com/apple-oss-distributions/xnu/blob/94d3b452840153a99b38a3a9659680b2a006908e/bsd/sys/socket.h#L1425-L1455
// [sendmsg_x]: https://github.com/apple-oss-distributions/xnu/blob/94d3b452840153a99b38a3a9659680b2a006908e/bsd/sys/socket.h#L1457-L1487
var FileAdapterForInterface = fileadapter.ForInterface[*PacketForwarder, net.PacketConn]

// MARK: - PacketForwarder for datagram file adapter

// PacketForwarder implements [fileadapter.PacketForwarder] for datagram file descriptor by using [recvmsg_x] and [sendmsg_x].
// See: [FileAdapterForInterface]
type PacketForwarder struct {
	localAddr         net.Addr
	receiveBufferSize int
	sendBufferSize    int
}

var _ fileadapter.PacketForwarder[net.PacketConn] = (*PacketForwarder)(nil)

// New creates a new [PacketForwarder].
func (*PacketForwarder) New() fileadapter.PacketForwarder[net.PacketConn] {
	return &PacketForwarder{}
}

// Sockopts returns [fileadapter.Sockopts] for the given [vmnet.Interface] and user desired options.
// Default values are based on the following references:
//   - https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment/maximumtransmissionunit?language=objc
func (*PacketForwarder) Sockopts(iface *vmnet.Interface, userOpts fileadapter.Sockopts) fileadapter.Sockopts {
	// Same as DatagramPacketForwarder
	return datagram.SockoptsForPacketConn(iface, userOpts)
}

// ConnAndFile creates a [net.PacketConn] and *[os.File] pair using [syscall.Socketpair].
func (f *PacketForwarder) ConnAndFile(opts fileadapter.Sockopts) (net.PacketConn, *os.File, error) {
	conn, file, err := datagram.PacketConnAndFile(opts)
	if err != nil {
		return nil, nil, err
	}
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	if rawConnControlErr := rawConn.Control(func(fd uintptr) {
		f.receiveBufferSize, err = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
		f.sendBufferSize, err = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	}); rawConnControlErr != nil {
		_ = conn.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("rawConn.Control failed: %w", rawConnControlErr)
	}
	if err != nil {
		_ = conn.Close()
		_ = file.Close()
		return nil, nil, fmt.Errorf("GetsockoptInt SO_RCVBUF failed: %w", err)
	}
	// Save local address for initialize msgHdrX
	f.localAddr = conn.LocalAddr()
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
		readMsgHdrsArray: newMsgHdrXArray(iface.MaxReadPacketCount, maxPacketSize, f.localAddr),
		sendBufferSize:   f.sendBufferSize,
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
		writeMsgHdrsArray: newMsgHdrXArray(iface.MaxWritePacketCount, maxPacketSize, f.localAddr),
	}
}

// MARK: - Interface -> Conn

// InterfaceToPacketConnForwarder forwards packets from [vmnet.Interface] to [net.PacketConn].
type InterfaceToPacketConnForwarder struct {
	readMsgHdrsArray *msgHdrXArray
	sendBufferSize   int
	packetCount      int
}

var _ fileadapter.InterfaceToConnForwarder[net.PacketConn] = (*InterfaceToPacketConnForwarder)(nil)

// ReadPacketsFromInterface reads packets from the [vmnet.Interface].
func (f *InterfaceToPacketConnForwarder) ReadPacketsFromInterface(iface *vmnet.Interface, estimatedCount int) (int, error) {
	f.readMsgHdrsArray.Reset()
	n, err := iface.ReadPackets(f.readMsgHdrsArray.pktDescsMgr.PktDescs, estimatedCount)
	f.packetCount = n
	return n, err
}

// WritePacketsToConn writes packets to the [net.PacketConn].
func (f *InterfaceToPacketConnForwarder) WritePacketsToConn(conn net.PacketConn) error {
	return f.readMsgHdrsArray.WritePacketsToPacketConn(conn, f.packetCount, f.sendBufferSize*2)
}

// MARK: - Conn -> Interface

// PacketConnToInterfaceForwarder forwards packets from [net.PacketConn] to [vmnet.Interface].
type PacketConnToInterfaceForwarder struct {
	writeMsgHdrsArray *msgHdrXArray
	packetCount       int
}

var _ fileadapter.ConnToInterfaceForwarder[net.PacketConn] = (*PacketConnToInterfaceForwarder)(nil)

// ReadPacketsFromConn reads packets from the [net.PacketConn].
func (f *PacketConnToInterfaceForwarder) ReadPacketsFromConn(conn net.PacketConn) error {
	n, err := f.writeMsgHdrsArray.ReadPacketsFromPacketConn(conn)
	f.packetCount = n
	return err
}

// WritePacketsToInterface writes packets to the [vmnet.Interface].
func (f *PacketConnToInterfaceForwarder) WritePacketsToInterface(iface *vmnet.Interface) error {
	return iface.WritePackets(f.writeMsgHdrsArray.pktDescsMgr.PktDescs, f.packetCount)
}

// MARK: - msgHdrXArray and its methods

// msgHdrX is a Go representation of [C.struct_msghdr_x].
type msgHdrX C.struct_msghdr_x

// msgHdrXArray manages an array of [msgHdrX] and its [PktDescsManager].
type msgHdrXArray struct {
	msgHdrs     *msgHdrX
	pktDescsMgr *vmnet.PktDescsManager
}

// newMsgHdrXArray allocates [msgHdrX] array and [PktDescsManager].
// The [msgHdrX]'s iov points to the [VMPktDesc]s' iov.
func newMsgHdrXArray(count int, maxPacketSize uint64, _ net.Addr) *msgHdrXArray {
	m := &msgHdrXArray{
		msgHdrs:     (*msgHdrX)(C.allocateMsgHdrXArray(C.int(count))),
		pktDescsMgr: vmnet.NewPktDescsManager(count, maxPacketSize),
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
		msgHdrX.msg_iov = (*C.struct_iovec)(pktDesc.GetVmPktIov())
		msgHdrX.msg_iovlen = 1
	}
	return m
}

// at returns the msgHdrX at index i.
func (m *msgHdrXArray) at(i int) *msgHdrX {
	return (*msgHdrX)(unsafe.Pointer(uintptr(unsafe.Pointer(m.msgHdrs)) + uintptr(i)*unsafe.Sizeof(msgHdrX{})))
}

// iter iterates over the [msgHdrXArray].
func (m *msgHdrXArray) iter(packetCount int) iter.Seq2[*msgHdrX, *vmnet.VMPktDesc] {
	return func(yield func(*msgHdrX, *vmnet.VMPktDesc) bool) {
		for i := range packetCount {
			if !yield(m.at(i), m.pktDescsMgr.At(i)) {
				return
			}
		}
	}
}

// iterFrom iterates over the [msgHdrXArray] from the given start index.
func (m *msgHdrXArray) iterFrom(start, packetCount int) iter.Seq2[*msgHdrX, *vmnet.VMPktDesc] {
	return func(yield func(*msgHdrX, *vmnet.VMPktDesc) bool) {
		for i := start; i < packetCount; i++ {
			if !yield(m.at(i), m.pktDescsMgr.At(i)) {
				return
			}
		}
	}
}

// reset resets the [vmnet.PktDescsManager] and updates msg_datalen for each [msgHdrX].
func (m *msgHdrXArray) Reset() {
	m.pktDescsMgr.Reset()
	m.clearDataLenAndFlags()
}

// clearDataLenAndFlags updates msg_datalen from msg_iov.iov_len for each [msgHdrX].
func (m *msgHdrXArray) clearDataLenAndFlags() {
	for msgHdrX := range m.iter(m.pktDescsMgr.MaxPacketCount()) {
		msgHdrX.msg_datalen = 0
		msgHdrX.msg_flags = 0
	}
}

// packetCountFitsInBatchSize returns the number of packets that can fit in the given batch size.
// It checks msg_iov.iov_len of each [msgHdrX].
// It starts from the given offset and checks up to packetCount.
func (m *msgHdrXArray) packetCountFitsInBatchSize(batchSize, offset, packetCount int) int {
	var fittedCount int
	var totalLen int
	for msgHdrX := range m.iterFrom(offset, packetCount) {
		totalLen += int(msgHdrX.msg_iov.iov_len)
		if totalLen > batchSize {
			break
		}
		fittedCount++
	}
	return fittedCount
}

// WritePacketsToPacketConn writes packets from the [msgHdrX]s to the [net.PacketConn].
//   - It returns an error if any occurs during sending packets.
func (m *msgHdrXArray) WritePacketsToPacketConn(conn net.PacketConn, packetCount, maximumBatchSize int) error {
	m.clearDataLenAndFlags()
	// Get rawConn for C.sendmsg_x
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var sentCount int
	var sendErr error
	rawConnWriteErr := rawConn.Write(func(fd uintptr) (done bool) {
		for sentCount < packetCount {
			// Limit batch size based on maximumBatchBytes
			batchCount := m.packetCountFitsInBatchSize(maximumBatchSize, sentCount, packetCount)
			// send packet from msgHdrX array
			n, err := C.sendmsg_x(C.int(fd), (*C.struct_msghdr_x)(m.at(sentCount)), C.u_int(batchCount), 0)
			if n < 0 {
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
					return false // try again later
				} else if errors.Is(err, syscall.ENOBUFS) {
					// Wait and try to send next packet
					// time.Sleep(100 * time.Microsecond)
					// continue
					return false
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

// ReadPacketsFromPacketConn reads packets from the [net.PacketConn] into [msgHdrX]s.
//   - It returns the number of packets read.
//   - The packets are read in batch by [recvmsg_x].
//   - It receives all available packets until no more packets are available, packetCount reaches maxPacketCount, or an error occurs.
//   - It waits for the connection to be ready for initial packet.
func (m *msgHdrXArray) ReadPacketsFromPacketConn(conn net.PacketConn) (int, error) {
	m.Reset()
	// Get rawConn for C.recvmsg_x
	var packetCount int
	// Get rawConn for C.recvmsg_x
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var recvErr error
	rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
		// receive packets into msgHdrXs (blocking)
		n, err := C.recvmsg_x(C.int(fd), (*C.struct_msghdr_x)(m.msgHdrs), C.u_int(m.pktDescsMgr.MaxPacketCount()), 0)
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
