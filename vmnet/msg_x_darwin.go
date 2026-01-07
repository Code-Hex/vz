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
	"unsafe"
)

// MARK: - DatagramNextFileAdaptorForInterface

// DatagramNextFileAdaptorForInterface returns a file for the given [Network].
//   - It uses [recvmsg_x] and [sendmsg_x] for packet transfer.
//   - Invoke the returned function in a separate goroutine to start packet forwarding between the vmnet interface and the file.
//   - The context can be used to stop the goroutines and the interface.
//   - The returned error channel can be used to receive errors from the goroutines.
//
// The returned file can be used as a file descriptor for QEMU's netdev datagram backend or VZ's [NewFileHandleNetworkDeviceAttachment]
// QEMU:
//
//	-netdev datagram,id=net0,addr.type=fd,addr.str=<file descriptor>
//
// VZ:
//
//	file, errCh, err := DatagramNextFileAdaptorForInterface(ctx, iface)
//	attachment := NewFileHandleNetworkDeviceAttachment(file)
//
// [recvmsg_x]: https://github.com/apple-oss-distributions/xnu/blob/94d3b452840153a99b38a3a9659680b2a006908e/bsd/sys/socket.h#L1425-L1455
// [sendmsg_x]: https://github.com/apple-oss-distributions/xnu/blob/94d3b452840153a99b38a3a9659680b2a006908e/bsd/sys/socket.h#L1457-L1487
var DatagramNextFileAdaptorForInterface = FileAdaptorForInterface[*DatagramNextPacketForwarder, net.PacketConn]

// MARK: - DatagramNextPacketForwarder for DatagramNext file adaptor

// DatagramNextPacketForwarder implements PacketForwarder for DatagramNext file descriptor.
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
	return iface.ReadPackets(f.readMsgHdrsArray.pkgDescsMgr.pktDescs, estimatedCount)
}

// WritePacketsToConn writes packets to the connection.
func (f *DatagramNextPacketForwarder) WritePacketsToConn(conn net.PacketConn, packetCount int) (int, error) {
	_, err := f.readMsgHdrsArray.writePacketsToPacketConn(conn, packetCount)
	if err != nil {
		return 0, err
	}
	return packetCount, nil
}

// ReadPacketsFromConn reads packets from the connection.
func (f *DatagramNextPacketForwarder) ReadPacketsFromConn(conn net.PacketConn) (int, error) {
	return f.writeMsgHdrsArray.readPacketsFromPacketConn(conn)
}

// WritePacketsToInterface writes packets to the vmnet Interface.
func (f *DatagramNextPacketForwarder) WritePacketsToInterface(iface *Interface, packetCount int) (int, error) {
	return iface.WritePackets(f.writeMsgHdrsArray.pkgDescsMgr.pktDescs, packetCount)
}

// MARK: - msgHdrXArray and its methods

// msgHdrX is a Go representation of C.struct_msghdr_x.
type msgHdrX C.struct_msghdr_x

// msgHdrXArray manages an array of msgHdrX and its pktDescsManager.
type msgHdrXArray struct {
	msgHdrs     *msgHdrX
	pkgDescsMgr *pktDescsManager
}

func newMsgHdrXArray(count int, maxPacketSize uint64, _ net.Addr) *msgHdrXArray {
	m := &msgHdrXArray{
		msgHdrs:     (*msgHdrX)(C.allocateMsgHdrXArray(C.int(count))),
		pkgDescsMgr: newPktDescsManager(count, maxPacketSize),
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
			if !yield(m.at(i), m.pkgDescsMgr.at(i)) {
				return
			}
		}
	}
}

// reset resets the pktDescsManager and updates msg_datalen for each msgHdrX.
func (m *msgHdrXArray) reset() {
	m.pkgDescsMgr.reset()
	m.clearDataLenAndFlags()
}

// clearDataLenAndFlags updates msg_datalen from msg_iov.iov_len for each msgHdrX.
func (m *msgHdrXArray) clearDataLenAndFlags() {
	for msgHdrX := range m.iter(m.pkgDescsMgr.maxPacketCount) {
		msgHdrX.msg_datalen = 0
		msgHdrX.msg_flags = 0
	}
}

func (m *msgHdrXArray) writePacketsToPacketConn(conn net.PacketConn, packetCount int) (int64, error) {
	m.clearDataLenAndFlags()
	// Get rawConn for C.sendmsg_x
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var sentCount int
	var sendmsgErr error
	for sentCount < packetCount {
		rawConnWriteErr := rawConn.Write(func(fd uintptr) (done bool) {
			n, err := C.sendmsg_x(C.int(fd), (*C.struct_msghdr_x)(m.at(sentCount)), C.u_int(packetCount-sentCount), 0)
			if n < 0 {
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.ENOBUFS) {
					return false // try again later
				}
				sendmsgErr = fmt.Errorf("sendmsg_x failed: %w", err)
				return true
			}
			sentCount += int(n)
			return true
		})
		if rawConnWriteErr != nil {
			return 0, rawConnWriteErr
		}
		if sendmsgErr != nil {
			return 0, sendmsgErr
		}
	}
	return int64(sentCount), nil
}

func (m *msgHdrXArray) readPacketsFromPacketConn(conn net.PacketConn) (int, error) {
	m.reset()
	// Get rawConn for C.recvmsg_x
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var packetCount int
	var recvmsgErr error
	rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
		n, err := C.recvmsg_x(C.int(fd), (*C.struct_msghdr_x)(m.msgHdrs), C.u_int(m.pkgDescsMgr.maxPacketCount), 0)
		if n < 0 {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.ENOBUFS) {
				return false // try again later
			}
			recvmsgErr = fmt.Errorf("recvmsg_x failed: %w", err)
			return true
		}
		packetCount = int(n)
		return true
	})
	if rawConnReadErr != nil {
		return 0, rawConnReadErr
	}
	if recvmsgErr != nil {
		return 0, recvmsgErr
	}
	for msgHdrX, pktDesc := range m.iter(packetCount) {
		// Update pktDesc's packet size from msg_iov.iov_len
		pktDesc.SetPacketSize(int(msgHdrX.msg_datalen))
	}
	return packetCount, nil
}
