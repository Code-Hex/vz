package vz_test

import (
	"bytes"
	"encoding/binary"
	"net/netip"
)

func echoReply(frame []byte, peerIP netip.Addr, peerMAC [6]byte) []byte {
	const ethernet = 14
	const ipv4 = 20
	if len(frame) < ethernet+ipv4+8 || !peerIP.Is4() || !bytes.Equal(frame[:6], peerMAC[:]) ||
		binary.BigEndian.Uint16(frame[12:14]) != 0x0800 || frame[ethernet] != 0x45 {
		return nil
	}
	length := int(binary.BigEndian.Uint16(frame[ethernet+2 : ethernet+4]))
	if length < ipv4+8 || length > len(frame)-ethernet || frame[ethernet+8] == 0 || frame[ethernet+9] != 1 ||
		binary.BigEndian.Uint16(frame[ethernet+6:ethernet+8])&0xbfff != 0 {
		return nil
	}
	ip := frame[ethernet : ethernet+ipv4]
	icmp := frame[ethernet+ipv4 : ethernet+length]
	address := peerIP.As4()
	if !bytes.Equal(ip[16:20], address[:]) || icmp[0] != 8 || icmp[1] != 0 ||
		internetChecksum(ip) != 0 || internetChecksum(icmp) != 0 {
		return nil
	}
	reply := bytes.Clone(frame[:ethernet+length])
	copy(reply[:6], frame[6:12])
	copy(reply[6:12], peerMAC[:])
	copy(reply[ethernet+12:ethernet+16], ip[16:20])
	copy(reply[ethernet+16:ethernet+20], ip[12:16])
	reply[ethernet+10], reply[ethernet+11] = 0, 0
	binary.BigEndian.PutUint16(reply[ethernet+10:ethernet+12], internetChecksum(reply[ethernet:ethernet+ipv4]))
	reply[ethernet+ipv4] = 0
	reply[ethernet+ipv4+2], reply[ethernet+ipv4+3] = 0, 0
	binary.BigEndian.PutUint16(reply[ethernet+ipv4+2:ethernet+ipv4+4], internetChecksum(reply[ethernet+ipv4:]))
	return reply
}

func internetChecksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data[:2]))
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}
