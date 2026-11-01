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
	frame, ip, udp    []byte
	version, ipHeader int
}

// The slices borrow frame. Only fixed IP headers can describe UDP L4 GSO here.
func parseUDPPacket(frame []byte) (udpPacket, error) {
	if len(frame) < ethernetHeaderSize {
		return udpPacket{}, fmt.Errorf("short Ethernet frame")
	}
	p := udpPacket{ip: frame[ethernetHeaderSize:]}
	switch binary.BigEndian.Uint16(frame[etherTypeOffset:]) {
	case etherTypeIPv4:
		if len(p.ip) < ipv4HeaderSize || p.ip[0]>>4 != ipv4Version || int(p.ip[0]&0x0f)*4 != ipv4HeaderSize {
			return udpPacket{}, fmt.Errorf("UDP GSO requires a fixed IPv4 header")
		}
		p.version, p.ipHeader = ipv4Version, ipv4HeaderSize
		n := int(binary.BigEndian.Uint16(p.ip[ipv4TotalLengthOffset:]))
		if n < ipv4HeaderSize+udpHeaderSize || n > len(p.ip) {
			return udpPacket{}, fmt.Errorf("invalid IPv4 UDP length")
		}
		p.ip = p.ip[:n]
		fragments := binary.BigEndian.Uint16(p.ip[ipv4FragmentFlagsOffset:])
		if p.ip[ipv4ProtocolOffset] != ipProtocolUDP || fragments&(ipv4MoreFragments|ipv4FragmentOffsetMask) != 0 {
			return udpPacket{}, fmt.Errorf("UDP GSO requires unfragmented UDP")
		}
	case etherTypeIPv6:
		if len(p.ip) < ipv6HeaderSize || p.ip[0]>>4 != ipv6Version {
			return udpPacket{}, fmt.Errorf("invalid IPv6 header")
		}
		p.version, p.ipHeader = ipv6Version, ipv6HeaderSize
		n := ipv6HeaderSize + int(binary.BigEndian.Uint16(p.ip[ipv6PayloadLengthOffset:]))
		if n < ipv6HeaderSize+udpHeaderSize || n > len(p.ip) {
			return udpPacket{}, fmt.Errorf("invalid IPv6 UDP length")
		}
		p.ip = p.ip[:n]
		if p.ip[ipv6NextHeaderOffset] != ipProtocolUDP {
			return udpPacket{}, fmt.Errorf("UDP GSO requires UDP without IPv6 extensions")
		}
	default:
		return udpPacket{}, fmt.Errorf("UDP GSO requires untagged IPv4 or IPv6")
	}
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
		udp := ip[p.ipHeader:]
		if p.version == ipv4Version {
			binary.BigEndian.PutUint16(ip[ipv4TotalLengthOffset:], uint16(len(ip)))
			id := binary.BigEndian.Uint16(p.ip[ipv4IdentificationOffset:]) + uint16(offset/size)
			binary.BigEndian.PutUint16(ip[ipv4IdentificationOffset:], id)
			binary.BigEndian.PutUint16(ip[ipv4ChecksumOffset:], 0)
			binary.BigEndian.PutUint16(ip[ipv4ChecksumOffset:], packetChecksum(ip[:ipv4HeaderSize], 0))
		} else {
			binary.BigEndian.PutUint16(ip[ipv6PayloadLengthOffset:], uint16(len(udp)))
		}
		binary.BigEndian.PutUint16(udp[udpLengthOffset:], uint16(len(udp)))
		binary.BigEndian.PutUint16(udp[udpChecksumOffset:], 0)
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
		if err := emit(frame); err != nil {
			return err
		}
	}
	return nil
}
