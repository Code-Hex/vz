package vmnetsock

import (
	"encoding/binary"
	"fmt"
)

// UDP's header and checksum layout follow RFC 768.
// https://www.rfc-editor.org/rfc/rfc768
const (
	ipProtocolUDP     = 17
	udpHeaderSize     = 8
	udpLengthOffset   = 4
	udpChecksumOffset = 6
)

type udpPacket struct {
	frame    []byte
	ip       []byte
	udp      []byte
	version  int
	ipHeader int
}

// The returned slices borrow frame.
func parseUDPPacket(frame []byte) (udpPacket, error) {
	if len(frame) < ethernetHeaderSize {
		return udpPacket{}, fmt.Errorf("short Ethernet frame")
	}
	var p udpPacket
	var err error
	switch binary.BigEndian.Uint16(frame[etherTypeOffset:]) {
	case etherTypeIPv4:
		p.ip, err = parseIPv4UDP(frame[ethernetHeaderSize:])
		p.version, p.ipHeader = ipv4Version, ipv4HeaderSize
	case etherTypeIPv6:
		p.ip, err = parseIPv6UDP(frame[ethernetHeaderSize:])
		p.version, p.ipHeader = ipv6Version, ipv6HeaderSize
	default:
		return udpPacket{}, fmt.Errorf("UDP GSO requires untagged IPv4 or IPv6")
	}
	if err != nil {
		return udpPacket{}, err
	}
	// IP length excludes Ethernet padding, which must not enter a UDP datagram.
	p.frame = frame[:ethernetHeaderSize+len(p.ip)]
	p.udp = p.ip[p.ipHeader:]
	if int(binary.BigEndian.Uint16(p.udp[udpLengthOffset:])) != len(p.udp) {
		return udpPacket{}, fmt.Errorf("UDP length does not match the IP payload")
	}
	if len(p.udp) == udpHeaderSize {
		return udpPacket{}, fmt.Errorf("UDP GSO requires UDP payload")
	}
	padding := len(frame) - len(p.frame)
	if padding > max(0, ethernetMinimumFrameSize-len(p.frame)) {
		return udpPacket{}, fmt.Errorf("UDP GSO frame length does not match the IP length")
	}
	return p, nil
}

func parseIPv4UDP(ip []byte) ([]byte, error) {
	if len(ip) < ipv4HeaderSize || ip[0]>>4 != ipv4Version {
		return nil, fmt.Errorf("UDP GSO requires a fixed IPv4 header")
	}
	headerSize := int(ip[0]&0x0f) * 4
	if headerSize != ipv4HeaderSize {
		return nil, fmt.Errorf("UDP GSO requires a fixed IPv4 header")
	}
	length := int(binary.BigEndian.Uint16(ip[ipv4TotalLengthOffset:]))
	if length < ipv4HeaderSize+udpHeaderSize || length > len(ip) {
		return nil, fmt.Errorf("invalid IPv4 UDP length")
	}
	fragments := binary.BigEndian.Uint16(ip[ipv4FragmentFlagsOffset:])
	if ip[ipv4ProtocolOffset] != ipProtocolUDP || fragments&(ipv4MoreFragments|ipv4FragmentOffsetMask) != 0 {
		return nil, fmt.Errorf("UDP GSO requires unfragmented UDP")
	}
	return ip[:length], nil
}

func parseIPv6UDP(ip []byte) ([]byte, error) {
	if len(ip) < ipv6HeaderSize || ip[0]>>4 != ipv6Version {
		return nil, fmt.Errorf("invalid IPv6 header")
	}
	length := ipv6HeaderSize + int(binary.BigEndian.Uint16(ip[ipv6PayloadLengthOffset:]))
	if length < ipv6HeaderSize+udpHeaderSize || length > len(ip) {
		return nil, fmt.Errorf("invalid IPv6 UDP length")
	}
	if ip[ipv6NextHeaderOffset] != ipProtocolUDP {
		return nil, fmt.Errorf("UDP GSO requires UDP without IPv6 extensions")
	}
	return ip[:length], nil
}

// emitSegments reuses its scratch frame after each callback. A consumer retaining
// a frame past the callback must copy it. Each output is a complete UDP datagram,
// even when size exceeds the remaining payload length.
func (p *udpPacket) emitSegments(size int, emit func([]byte) error) error {
	headers := ethernetHeaderSize + p.ipHeader + udpHeaderSize
	buffer := make([]byte, headers+size)
	payload := p.udp[udpHeaderSize:]
	for offset := 0; offset < len(payload); offset += size {
		end := min(offset+size, len(payload))
		frame := buffer[:headers+end-offset]
		copy(frame, p.frame[:headers])
		copy(frame[headers:], payload[offset:end])
		ip := frame[ethernetHeaderSize:]
		p.updateIPHeader(ip, offset/size)
		p.updateUDPHeader(ip)
		if err := emit(frame); err != nil {
			return err
		}
	}
	return nil
}

func (p *udpPacket) updateIPHeader(ip []byte, segmentIndex int) {
	if p.version == ipv6Version {
		binary.BigEndian.PutUint16(ip[ipv6PayloadLengthOffset:], uint16(len(ip)-ipv6HeaderSize))
		return
	}
	binary.BigEndian.PutUint16(ip[ipv4TotalLengthOffset:], uint16(len(ip)))
	id := binary.BigEndian.Uint16(p.ip[ipv4IdentificationOffset:]) + uint16(segmentIndex)
	binary.BigEndian.PutUint16(ip[ipv4IdentificationOffset:], id)
	binary.BigEndian.PutUint16(ip[ipv4ChecksumOffset:], 0)
	binary.BigEndian.PutUint16(ip[ipv4ChecksumOffset:], packetChecksum(ip[:ipv4HeaderSize], 0))
}

func (p *udpPacket) updateUDPHeader(ip []byte) {
	udp := ip[p.ipHeader:]
	binary.BigEndian.PutUint16(udp[udpLengthOffset:], uint16(len(udp)))
	binary.BigEndian.PutUint16(udp[udpChecksumOffset:], 0)
	// The UDP length also appears in the checksum pseudo-header.
	sum := uint64(ipProtocolUDP + len(udp))
	if p.version == ipv4Version {
		sum = packetSum(ip[ipv4SourceOffset:ipv4HeaderSize], sum)
	} else {
		sum = packetSum(ip[ipv6SourceOffset:ipv6HeaderSize], sum)
	}
	checksum := packetChecksum(udp, sum)
	// RFC 768 encodes a computed zero as all ones.
	if checksum == 0 {
		checksum = 0xffff
	}
	binary.BigEndian.PutUint16(udp[udpChecksumOffset:], checksum)
}
