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
	"iter"
	"net"
	"runtime"
	"slices"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const HeaderSizeForStream = int(unsafe.Sizeof(C.uint32_t(0)))
const VirtioNetHdrSize = 12 // Size of virtio_net_hdr_v1

// MARK: - VMPktDesc

// VMPktDesc is a Go representation of C.struct_vmpktdesc.
type VMPktDesc C.struct_vmpktdesc

// SetPacketSize sets the packet size in VMPktDesc.
func (v *VMPktDesc) SetPacketSize(size int) *VMPktDesc {
	v.vm_pkt_size = C.size_t(size)
	v.vm_pkt_iov.iov_len = C.size_t(size)
	return v
}

// GetPacketSize gets the packet size from VMPktDesc.
func (v *VMPktDesc) GetPacketSize() int {
	return int(v.vm_pkt_size)
}

// GetVmPktIov returns the vm_pkt_iov pointer in VMPktDesc.
func (v *VMPktDesc) GetVmPktIov() unsafe.Pointer {
	return unsafe.Pointer(v.vm_pkt_iov)
}

// MARK: - PktDescsManager

// PktDescsManager manages pktDescs and their backing buffers.
type PktDescsManager struct {
	PktDescs       *VMPktDesc
	backingBuffers net.Buffers
	writingBuffers net.Buffers
	maxPacketSize  uint64
}

// NewPktDescsManager allocates pktDesc array and backing buffers.
// pktDesc's iov_base points to the buffer after 4-byte header.
// The 4-byte header is reserved for packet size to the connection.
func NewPktDescsManager(count int, maxPacketSize uint64) *PktDescsManager {
	v := &PktDescsManager{
		PktDescs:       (*VMPktDesc)(C.allocateVMPktDescArray(C.int(count), C.uint64_t(maxPacketSize))),
		backingBuffers: make(net.Buffers, 0, count),
		maxPacketSize:  maxPacketSize,
	}
	runtime.AddCleanup(v, func(self *C.struct_vmpktdesc) { C.deallocateVMPktDescArray(self) }, (*C.struct_vmpktdesc)(v.PktDescs))
	bufLen := maxPacketSize + uint64(HeaderSizeForStream)
	// Allocate a single block of memory for all buffers
	unsafeBuffers := C.malloc(C.size_t(bufLen) * C.size_t(count))
	runtime.AddCleanup(v, func(ptr unsafe.Pointer) { C.free(ptr) }, unsafeBuffers)
	for i := range count {
		unsafeBuffer := unsafe.Add(unsafeBuffers, C.size_t(bufLen)*C.size_t(i))
		vmPktDesc := v.At(i)
		// point after the 4-byte header
		vmPktDesc.vm_pkt_iov.iov_base = unsafe.Add(unsafeBuffer, HeaderSizeForStream)
		vmPktDesc.vm_pkt_iov.iov_len = C.size_t(maxPacketSize)
		buf := unsafe.Slice((*byte)(unsafeBuffer), bufLen)
		v.backingBuffers = append(v.backingBuffers, buf)
	}
	v.writingBuffers = slices.Clone(v.backingBuffers)
	return v
}

// at returns the pointer to the pktDesc at the given index.
func (v *PktDescsManager) At(index int) *VMPktDesc {
	return (*VMPktDesc)(unsafe.Add(unsafe.Pointer(v.PktDescs), index*int(unsafe.Sizeof(VMPktDesc{}))))
}

// headerBufferAt returns the 4-byte header buffer at the given index.
func (v *PktDescsManager) headerBufferAt(index int) []byte {
	return v.backingBuffers[index][:HeaderSizeForStream]
}

// iter iterates over pktDescs and their corresponding buffers.
func (v *PktDescsManager) iter(packetCount int) iter.Seq2[int, *VMPktDesc] {
	return func(yield func(int, *VMPktDesc) bool) {
		for i := range packetCount {
			if !yield(i, v.At(i)) {
				return
			}
		}
	}
}

// maxPacketCount returns the maximum number of pktDescs managed.
func (v *PktDescsManager) MaxPacketCount() int {
	return len(v.backingBuffers)
}

// packetBufferAt returns the packet buffer at the given index and offset.
func (v *PktDescsManager) packetBufferAt(index, offset int) []byte {
	end := HeaderSizeForStream + v.At(index).GetPacketSize()
	return v.backingBuffers[index][HeaderSizeForStream+offset : end]
}

// Reset resets pktDescs to initial state.
func (v *PktDescsManager) Reset() {
	C.resetVMPktDescArray((*C.struct_vmpktdesc)(v.PktDescs), C.int(v.MaxPacketCount()), C.uint64_t(v.maxPacketSize))
}

// MARK: - PktDescsManager methods for datagram file adaptor

// buffersForWritingToPacketConn returns [net.Buffers] to write to the [net.PacketConn]
// adjusted their buffer sizes based vm_pkt_size in [VMPktDesc]s read from [Interface].
// The 4-byte header is excluded.
func (v *PktDescsManager) buffersForWritingToPacketConn(packetCount int) (net.Buffers, error) {
	for i, vmPktDesc := range v.iter(packetCount) {
		if uint64(vmPktDesc.GetPacketSize()) > v.maxPacketSize {
			return nil, fmt.Errorf("vm_pkt_size %d exceeds maxPacketSize %d", vmPktDesc.GetPacketSize(), v.maxPacketSize)
		}
		// Resize buffer to exclude the 4-byte header
		v.writingBuffers[i] = v.packetBufferAt(i, 0)
	}
	return v.writingBuffers[:packetCount], nil
}

// WritePacketsToPacketConn writes packets from [VMPktDesc]s to the [net.PacketConn].
//   - It returns an error if any occurs during sending packets.
func (v *PktDescsManager) WritePacketsToPacketConn(conn net.PacketConn, packetCount int) error {
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
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
					return false // try again later
				} else if errors.Is(err, syscall.ENOBUFS) {
					// Wait and try to send next packet
					// time.Sleep(100 * time.Microsecond)
					// continue
					return false
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

// ReadPacketsFromPacketConn reads packets from the [net.PacketConn] into [VMPktDesc]s.
//   - It returns the number of packets read.
//   - The packets are expected to come one by one.
//   - It receives all available packets until no more packets are available, packetCount reaches maxPacketCount, or an error occurs.
//   - It waits for the connection to be ready for initial packet.
func (v *PktDescsManager) ReadPacketsFromPacketConn(conn net.PacketConn) (int, error) {
	// Get rawConn for syscall.Recvfrom
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	var packetCount int
	var recvErr error
	rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
		// Read available packets until no more packets are available or packetCount reaches maxPacketCount
		for packetCount < v.MaxPacketCount() {
			// receive packet into buffer
			n, _, err := syscall.Recvfrom(int(fd), v.backingBuffers[packetCount][HeaderSizeForStream:], 0)
			if err != nil {
				if errors.Is(err, syscall.EAGAIN) {
					// Retry if no packets have been received yet
					return packetCount > 0
				}
				recvErr = fmt.Errorf("syscall.Recvfrom failed: %w", err)
				return true
			}
			// Set packet size in pktDesc
			v.At(packetCount).SetPacketSize(n)
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

// MARK: - PktDescsManager methods for stream file adaptor

// buffersForWritingToConn returns [net.Buffers] to write to the [net.Conn]
// adjusted their buffer sizes based vm_pkt_size in [VMPktDesc]s read from [Interface].
func (v *PktDescsManager) buffersForWritingToConn(packetCount int) (net.Buffers, error) {
	for i, vmPktDesc := range v.iter(packetCount) {
		if uint64(vmPktDesc.GetPacketSize()) > v.maxPacketSize {
			return nil, fmt.Errorf("vm_pkt_size %d exceeds maxPacketSize %d", vmPktDesc.GetPacketSize(), v.maxPacketSize)
		}
		// Write packet size to the 4-byte header
		binary.BigEndian.PutUint32(v.headerBufferAt(i), uint32(vmPktDesc.GetPacketSize()))
		// Resize buffer to include header and packet size
		v.writingBuffers[i] = v.backingBuffers[i][:HeaderSizeForStream+vmPktDesc.GetPacketSize()]
	}
	return v.writingBuffers[:packetCount], nil
}

func bufferCountFitsInReceiveBuffer(buffers net.Buffers, receiveBufferSize int) int {
	totalSize := 0
	for i, buf := range buffers {
		totalSize += len(buf)
		if totalSize > receiveBufferSize {
			return i
		}
	}
	return len(buffers)
}

// WritePacketsToConn writes packets from [VMPktDesc]s to the [net.Conn].
//   - It returns the number of bytes written.
func (v *PktDescsManager) WritePacketsToConn(conn net.Conn, packetCount, receiveBufferSize int) error {
	// To use built-in Writev implementation in net package (internal/poll.FD.Writev),
	// we use net.Buffers and its WriteTo method.
	buffers, err := v.buffersForWritingToConn(packetCount)
	if err != nil {
		return fmt.Errorf("buffersForWritingToConn failed: %w", err)
	}
	var offset int
	for offset < packetCount {
		// Limit buffers to fit in receive buffer size
		fitCount := bufferCountFitsInReceiveBuffer(buffers[offset:], receiveBufferSize)
		limitedBuffers := buffers[offset : offset+fitCount]
		// Write packets to the connection
		// [Buffers.WriteTo] uses writev syscall internally, it also handles partial writes until all data is written.
		// So, we don't need to handle partial writes here.
		_, err = limitedBuffers.WriteTo(conn)
		if err != nil {
			return fmt.Errorf("buffers.WriteTo failed: %w", err)
		}
		offset += fitCount
	}
	return nil
}

// buffersForReadingFromConn returns [net.Buffers] to read from the [net.Conn]
// for the given index and offset.
// It prepares buffer for the next header read as well if possible.
func (v *PktDescsManager) buffersForReadingFromConn(index, offset int) net.Buffers {
	if offset < v.At(index).GetPacketSize() {
		if index+1 < v.MaxPacketCount() {
			// prepare next header read as well
			return net.Buffers{
				v.packetBufferAt(index, offset),
				v.headerBufferAt(index + 1),
			}
		}
		return net.Buffers{
			v.packetBufferAt(index, offset),
		}
	}
	headerOffset := offset - v.At(index).GetPacketSize()
	return net.Buffers{
		v.headerBufferAt(index)[headerOffset:],
	}
}

// ReadPacketsFromConn reads packets from the [net.Conn] into [VMPktDesc]s.
//   - It returns the number of packets read.
//   - The packets are expected to come one by one with 4-byte big-endian header indicating the packet size.
//   - It reads all available packets until no more packets are available, packetCount reaches maxPacketCount, or an error occurs.
//   - It waits for the connection to be ready for initial read of 4-byte header.
func (v *PktDescsManager) ReadPacketsFromConn(conn net.Conn) (int, error) {
	var packetCount int
	// Wait until 4-byte header is read
	if _, err := conn.Read(v.headerBufferAt(packetCount)); err != nil {
		return 0, fmt.Errorf("conn.Read failed: %w", err)
	}
	// Get rawConn for Readv
	rawConn, _ := conn.(syscall.Conn).SyscallConn()
	// Read available packets
	for {
		packetLen := int(binary.BigEndian.Uint32(v.headerBufferAt(packetCount)))
		if packetLen == 0 || uint64(packetLen) > v.maxPacketSize {
			return 0, fmt.Errorf("invalid packetLen: %d (max %d)", packetLen, v.maxPacketSize)
		}
		v.At(packetCount).SetPacketSize(packetLen)

		// Read packet from the connection until full packet is read, including next header if possible.
		var bytesHasBeenRead int
		var readErr error
		if rawConnReadErr := rawConn.Read(func(fd uintptr) (done bool) {
			bufs := v.buffersForReadingFromConn(packetCount, bytesHasBeenRead)
			// read packet into buffers
			n, err := unix.Readv(int(fd), bufs)
			if n <= 0 {
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EWOULDBLOCK) {
					return false // try again later
				}
				readErr = fmt.Errorf("unix.Readv failed: %w", err)
				return true
			}
			bytesHasBeenRead += n
			if bytesHasBeenRead == packetLen+HeaderSizeForStream || bytesHasBeenRead == packetLen {
				return true
			}
			// Partial read, read again
			return false
		}); rawConnReadErr != nil {
			return 0, fmt.Errorf("rawConn.Read failed: %w", rawConnReadErr)
		}
		if readErr != nil {
			return 0, fmt.Errorf("closure in rawConn.Read failed: %w", readErr)
		}
		packetCount++
		if bytesHasBeenRead == packetLen+HeaderSizeForStream {
			// next packet header is also read, continue to read next packet
		} else if bytesHasBeenRead == packetLen {
			// next packet seems not available now, or reached maxPacketCount
			break
		}
	}
	return packetCount, nil
}
