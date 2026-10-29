package vmnetsock

import (
	"encoding/binary"
	"fmt"
)

// vmnet uses the 12-byte Virtio 1.1 network header, including num_buffers.
// See https://developer.apple.com/documentation/vmnet/vmnet_enable_virtio_header_key
const virtioHeaderSize = 12

const (
	virtioNeedsChecksum byte = 1
	virtioRSCInfo       byte = 4
)

// virtioEthernetPacket completes checksum offload in place and returns the
// Ethernet frame. The caller retains the original slice for the next read.
func virtioEthernetPacket(packet []byte) ([]byte, error) {
	if len(packet) < virtioHeaderSize+14 {
		return nil, fmt.Errorf("virtio packet is shorter than its header and Ethernet frame")
	}
	flags := packet[0]
	if packet[1] != 0 {
		return nil, fmt.Errorf("unsupported virtio GSO type %#x", packet[1])
	}
	frame := packet[virtioHeaderSize:]
	// Virtio 1.1 section 5.1.6.4.2 requires ignoring unknown flags.
	// Checksum fields apply only when NEEDS_CSUM is set without RSC_INFO.
	// See https://docs.oasis-open.org/virtio/virtio/v1.1/virtio-v1.1.html
	if flags&virtioNeedsChecksum == 0 {
		return frame, nil
	}
	if flags&virtioRSCInfo != 0 {
		return nil, fmt.Errorf("cannot complete virtio checksum with RSC metadata")
	}
	start := int(binary.LittleEndian.Uint16(packet[6:8]))
	offset := int(binary.LittleEndian.Uint16(packet[8:10]))
	if start+offset+2 > len(frame) {
		return nil, fmt.Errorf("virtio checksum field lies outside the Ethernet frame")
	}

	// NEEDS_CSUM includes the pseudo-header seed in the checksum field.
	// Sum it with the payload, then replace it with the completed checksum.
	// See https://docs.oasis-open.org/virtio/virtio/v1.1/virtio-v1.1.html
	var sum uint64
	data := frame[start:]
	for len(data) >= 2 {
		sum += uint64(binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	if len(data) != 0 {
		sum += uint64(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	checksum := ^uint16(sum)
	if checksum == 0 {
		checksum = 0xffff
	}
	binary.BigEndian.PutUint16(frame[start+offset:], checksum)
	return frame, nil
}
