package vz_test

import (
	"bytes"
	"encoding/binary"
	"net/netip"
)

func arpReply(frame []byte, peerIP netip.Addr, peerMAC [6]byte) []byte {
	if len(frame) < 42 || !peerIP.Is4() || binary.BigEndian.Uint16(frame[12:14]) != 0x0806 ||
		binary.BigEndian.Uint16(frame[14:16]) != 1 || binary.BigEndian.Uint16(frame[16:18]) != 0x0800 ||
		frame[18] != 6 || frame[19] != 4 || binary.BigEndian.Uint16(frame[20:22]) != 1 ||
		!bytes.Equal(frame[6:12], frame[22:28]) {
		return nil
	}
	broadcast := [6]byte{255, 255, 255, 255, 255, 255}
	address := peerIP.As4()
	if (!bytes.Equal(frame[:6], peerMAC[:]) && !bytes.Equal(frame[:6], broadcast[:])) ||
		!bytes.Equal(frame[38:42], address[:]) {
		return nil
	}
	reply := make([]byte, 60)
	copy(reply[:6], frame[6:12])
	copy(reply[6:12], peerMAC[:])
	copy(reply[12:20], frame[12:20])
	binary.BigEndian.PutUint16(reply[20:22], 2)
	copy(reply[22:28], peerMAC[:])
	copy(reply[28:32], address[:])
	copy(reply[32:38], frame[22:28])
	copy(reply[38:42], frame[28:32])
	return reply
}
