package vmnet

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework vmnet
# include "vmnet_darwin.h"
*/
import "C"
import (
	"iter"
	"net"
	"runtime"
	"slices"
	"unsafe"
)

const headerSize = unsafe.Sizeof(C.uint32_t(0))
const virtioNetHdrSize = 12 // Size of virtio_net_hdr_v1

// VMPktDesc is a Go representation of C.struct_vmpktdesc.
type VMPktDesc C.struct_vmpktdesc

// SetPacketSize sets the packet size in VMPktDesc.
func (v *VMPktDesc) SetPacketSize(size int) {
	v.vm_pkt_size = C.size_t(size)
	v.vm_pkt_iov.iov_len = C.size_t(size)
}

// pktDescsManager manages pktDescs and their backing buffers.
type pktDescsManager struct {
	pktDescs       *VMPktDesc
	backingBuffers net.Buffers
	writingBuffers net.Buffers
	maxPacketCount int
	maxPacketSize  uint64
}

// newPktDescsManager allocates pktDesc array and backing buffers.
// pktDesc's iov_base points to the buffer after 4-byte header.
// The 4-byte header is reserved for packet size to the connection.
func newPktDescsManager(count int, maxPacketSize uint64) *pktDescsManager {
	v := &pktDescsManager{
		pktDescs:       (*VMPktDesc)(C.allocateVMPktDescArray(C.int(count), C.uint64_t(maxPacketSize))),
		backingBuffers: make(net.Buffers, 0, count),
		maxPacketCount: count,
		maxPacketSize:  maxPacketSize,
	}
	runtime.AddCleanup(v, func(self *C.struct_vmpktdesc) { C.deallocateVMPktDescArray(self) }, (*C.struct_vmpktdesc)(v.pktDescs))
	bufLen := maxPacketSize + uint64(headerSize)
	for i := range count {
		// Allocate buffer with extra 4 bytes for header
		buf := make([]byte, bufLen)
		vmPktDesc := v.at(i)
		// point after the 4-byte header
		vmPktDesc.vm_pkt_iov.iov_base = unsafe.Add(unsafe.Pointer(unsafe.SliceData(buf)), headerSize)
		vmPktDesc.vm_pkt_iov.iov_len = C.size_t(maxPacketSize)
		v.backingBuffers = append(v.backingBuffers, buf)
	}
	v.writingBuffers = slices.Clone(v.backingBuffers)
	return v
}

// at returns the pointer to the pktDesc at the given index.
func (v *pktDescsManager) at(index int) *VMPktDesc {
	return (*VMPktDesc)(unsafe.Add(unsafe.Pointer(v.pktDescs), index*int(unsafe.Sizeof(VMPktDesc{}))))
}

// iter iterates over pktDescs and their corresponding buffers.
func (v *pktDescsManager) iter(packetCount int) iter.Seq2[int, *VMPktDesc] {
	return func(yield func(int, *VMPktDesc) bool) {
		for i := range packetCount {
			if !yield(i, v.at(i)) {
				return
			}
		}
	}
}

// reset resets pktDescs to initial state.
func (v *pktDescsManager) reset() {
	C.resetVMPktDescArray((*C.struct_vmpktdesc)(v.pktDescs), C.int(v.maxPacketCount), C.uint64_t(v.maxPacketSize))
}
