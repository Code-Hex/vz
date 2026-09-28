package vmnet

/*
#include "vmnet_darwin.h"
*/
import "C"

import (
	"fmt"
	"math"
	"runtime"
	"unsafe"
)

// PktDescsManager owns the descriptors and packet memory used by vmnet I/O.
// A manager must not be used concurrently.
type PktDescsManager struct {
	packets       *C.struct_vmpktdesc
	buffer        unsafe.Pointer
	count         int
	maxPacketSize uint64
	valid         []bool
}

// NewPktDescsManager allocates count packet buffers of maxPacketSize bytes.
func NewPktDescsManager(count int, maxPacketSize uint64) (*PktDescsManager, error) {
	if count <= 0 || count > math.MaxInt32 || maxPacketSize == 0 {
		return nil, fmt.Errorf("invalid packet count %d or size %d", count, maxPacketSize)
	}
	maxSize := uint64(int(^uint(0) >> 1))
	if maxPacketSize > maxSize || uint64(count) > maxSize/maxPacketSize {
		return nil, fmt.Errorf("packet allocation exceeds addressable memory")
	}
	descriptorSize := uint64(C.sizeof_struct_vmpktdesc + C.sizeof_struct_iovec)
	if uint64(count) > maxSize/descriptorSize {
		return nil, fmt.Errorf("descriptor allocation exceeds addressable memory")
	}
	packets := C.allocateVMPktDescArray(C.int(count))
	if packets == nil {
		return nil, fmt.Errorf("allocate packet descriptors: out of memory")
	}
	buffer := C.malloc(C.size_t(uint64(count) * maxPacketSize))
	if buffer == nil {
		C.free(unsafe.Pointer(packets))
		return nil, fmt.Errorf("allocate packet buffers: out of memory")
	}
	C.initializeVMPktDescArray(packets, C.int(count), C.size_t(maxPacketSize), buffer)
	manager := &PktDescsManager{
		packets:       packets,
		buffer:        buffer,
		count:         count,
		maxPacketSize: maxPacketSize,
		valid:         make([]bool, count),
	}
	runtime.AddCleanup(manager, func(allocation struct{ packets, buffer unsafe.Pointer }) {
		C.free(allocation.packets)
		C.free(allocation.buffer)
	}, struct{ packets, buffer unsafe.Pointer }{unsafe.Pointer(packets), buffer})
	return manager, nil
}

// MaxPacketCount reports the number of descriptors owned by the manager.
func (m *PktDescsManager) MaxPacketCount() int { return m.count }

// SetPacket copies a packet into a descriptor for WritePackets.
func (m *PktDescsManager) SetPacket(index int, packet []byte) error {
	if m == nil || index < 0 || index >= m.count {
		return fmt.Errorf("packet index %d out of range", index)
	}
	if len(packet) == 0 || uint64(len(packet)) > m.maxPacketSize {
		return fmt.Errorf("packet size %d outside range 1..%d", len(packet), m.maxPacketSize)
	}
	desc := &unsafe.Slice(m.packets, m.count)[index]
	copy(unsafe.Slice((*byte)(desc.vm_pkt_iov.iov_base), len(packet)), packet)
	desc.vm_pkt_size = C.size_t(len(packet))
	desc.vm_pkt_iov.iov_len = C.size_t(len(packet))
	m.valid[index] = true
	runtime.KeepAlive(m)
	return nil
}

// Packet returns a copy of a packet read from vmnet or provided with SetPacket.
func (m *PktDescsManager) Packet(index int) ([]byte, error) {
	if m == nil || index < 0 || index >= m.count {
		return nil, fmt.Errorf("packet index %d out of range", index)
	}
	if !m.valid[index] {
		return nil, fmt.Errorf("packet %d has no data", index)
	}
	desc := &unsafe.Slice(m.packets, m.count)[index]
	if uint64(desc.vm_pkt_size) > m.maxPacketSize {
		return nil, fmt.Errorf("packet %d exceeds maximum size", index)
	}
	packet := make([]byte, int(desc.vm_pkt_size))
	copy(packet, unsafe.Slice((*byte)(desc.vm_pkt_iov.iov_base), len(packet)))
	runtime.KeepAlive(m)
	return packet, nil
}
