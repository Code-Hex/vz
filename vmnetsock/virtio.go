package vmnetsock

import (
	"encoding/binary"
	"fmt"
	"time"
)

// vmnet uses the 12-byte Virtio 1.1 network header, including num_buffers.
// See https://developer.apple.com/documentation/vmnet/vmnet_enable_virtio_header_key
const virtioHeaderSize = 12

const (
	virtioNeedsChecksum byte = 1
	virtioDataValid     byte = 2
	virtioRSCInfo       byte = 4
	virtioGSOTCPv4      byte = 1
	virtioGSOTCPv6      byte = 4
	virtioGSOUDPL4      byte = 5
	virtioGSOECN        byte = 0x80
)

const (
	virtioGSOSizeOffset       = 4
	virtioChecksumStartOffset = 6
	virtioChecksumFieldOffset = 8

	virtioRSCSegmentsOffset      = 6
	virtioRSCDuplicateACKsOffset = 8
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
	start := int(binary.LittleEndian.Uint16(packet[virtioChecksumStartOffset:]))
	offset := int(binary.LittleEndian.Uint16(packet[virtioChecksumFieldOffset:]))
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

func forwardVirtioUDPGSO(packet []byte, mtu int, emit func([]byte) error) error {
	if len(packet) < virtioHeaderSize+ethernetHeaderSize {
		return fmt.Errorf("virtio packet is shorter than its header and Ethernet frame")
	}
	if packet[1] != virtioGSOUDPL4 || packet[0]&virtioRSCInfo != 0 {
		return fmt.Errorf("unsupported virtio UDP GSO metadata")
	}
	if packet[0]&virtioNeedsChecksum == 0 {
		return fmt.Errorf("virtio UDP GSO requires partial checksum metadata")
	}
	p, err := parseUDPPacket(packet[virtioHeaderSize:])
	if err != nil {
		return err
	}
	start := int(binary.LittleEndian.Uint16(packet[virtioChecksumStartOffset:]))
	offset := int(binary.LittleEndian.Uint16(packet[virtioChecksumFieldOffset:]))
	if start != ethernetHeaderSize+p.ipHeader || offset != udpChecksumOffset {
		return fmt.Errorf("virtio UDP GSO checksum metadata does not match the UDP header")
	}
	size := int(binary.LittleEndian.Uint16(packet[virtioGSOSizeOffset:]))
	// UDP L4 GSO describes datagram boundaries. Reducing size to fit the MTU
	// would change those boundaries, unlike a TCP stream's segmentation.
	if size == 0 || size+p.ipHeader+udpHeaderSize > mtu {
		return fmt.Errorf("virtio UDP GSO segment size does not fit the MTU")
	}
	return p.emitSegments(size, emit)
}

func (s *tcpSegmentation) forwardVirtio(packet []byte, now time.Time, emit func([]byte) error) error {
	if len(packet) < virtioHeaderSize+ethernetHeaderSize {
		return fmt.Errorf("virtio packet is shorter than its header and Ethernet frame")
	}
	rsc := packet[0]&virtioRSCInfo != 0
	if packet[1] == 0 && !rsc {
		frame, err := virtioEthernetPacket(packet)
		if err != nil {
			return err
		}
		return s.forwardNative(frame, now, emit)
	}

	var expectedVersion int
	switch packet[1] &^ virtioGSOECN {
	case 0:
		if packet[1] != 0 {
			return fmt.Errorf("unsupported virtio GSO type %#x", packet[1])
		}
	case virtioGSOTCPv4:
		expectedVersion = ipv4Version
	case virtioGSOTCPv6:
		expectedVersion = ipv6Version
	default:
		return fmt.Errorf("unsupported virtio GSO type %#x", packet[1])
	}
	size := int(binary.LittleEndian.Uint16(packet[virtioGSOSizeOffset:]))
	if expectedVersion != 0 && size == 0 {
		return fmt.Errorf("virtio TCP GSO segment size is zero")
	}
	frame := packet[virtioHeaderSize:]
	p, err := parseTCPPacket(frame)
	if err != nil {
		return err
	}
	if p.tcp == nil {
		// vmnet also marks ordinary non-TCP packets with RSC_INFO. Keep the
		// existing Ethernet path, which ignores counters without NEEDS_CSUM.
		if rsc && expectedVersion == 0 {
			frame, err := virtioEthernetPacket(packet)
			if err != nil {
				return err
			}
			return s.forwardNative(frame, now, emit)
		}
		return fmt.Errorf("virtio TCP GSO requires TCP payload")
	}
	if p.payload() == 0 && (!rsc || expectedVersion != 0) {
		return fmt.Errorf("virtio TCP GSO requires TCP payload")
	}
	if rsc {
		if packet[0]&(virtioNeedsChecksum|virtioDataValid) == 0 {
			return fmt.Errorf("virtio TCP RSC requires checksum validation metadata")
		}
		// RSC reuses csum_offset for a duplicate ACK count. One TCP header
		// cannot reconstruct those ACKs, so do not silently discard them.
		// See Virtio 1.2 section 5.1.6.4.
		// https://docs.oasis-open.org/virtio/virtio/v1.2/virtio-v1.2.html
		if binary.LittleEndian.Uint16(packet[virtioRSCDuplicateACKsOffset:]) != 0 {
			return fmt.Errorf("virtio TCP RSC duplicate ACKs are not supported")
		}
	}
	if (!rsc || expectedVersion != 0) && packet[0]&virtioNeedsChecksum == 0 {
		return fmt.Errorf("virtio TCP GSO requires partial checksum metadata")
	}
	padding := len(frame) - len(p.frame)
	maxPadding := max(0, ethernetMinimumFrameSize-len(p.frame))
	if padding > maxPadding {
		return fmt.Errorf("virtio TCP GSO frame length does not match the IP length")
	}
	if expectedVersion != 0 && p.version != expectedVersion {
		return fmt.Errorf("virtio GSO type does not match the IP version")
	}
	if !rsc {
		start := int(binary.LittleEndian.Uint16(packet[virtioChecksumStartOffset:]))
		offset := int(binary.LittleEndian.Uint16(packet[virtioChecksumFieldOffset:]))
		if start != ethernetHeaderSize+p.ipHeader || offset != tcpChecksumOffset {
			return fmt.Errorf("virtio TCP GSO checksum metadata does not match the TCP header")
		}
	}

	mss, err := s.observe(&p, false, now)
	if err != nil {
		return err
	}
	// gso_size counts payload bytes, unlike MSS, which also allows TCP options.
	// hdr_len is only a hint; use the parsed headers for bounds and checksums.
	// See Virtio 1.1 sections 5.1.6.2 and 5.1.6.4.
	// https://docs.oasis-open.org/virtio/virtio/v1.1/virtio-v1.1.html
	// RSC counters do not describe the original segment boundaries. In
	// particular, vmnet can return RSC with GSO_NONE and no segment size.
	// Repacketize using the observed receiver MSS and the interface MTU.
	limit := s.mtu - p.ipHeader - p.tcpHeader
	if p.payload() != 0 {
		limit = min(limit, mss-(p.tcpHeader-tcpHeaderSize))
	}
	if expectedVersion != 0 {
		limit = min(limit, size)
	}
	if limit <= 0 {
		return fmt.Errorf("TCP headers leave no payload within GSO, MSS and MTU limits")
	}
	return p.emitSegments(limit, emit)
}
