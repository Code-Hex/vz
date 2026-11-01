package vmnetsock

import "encoding/binary"

func packetSum(data []byte, sum uint64) uint64 {
	for len(data) >= 2 {
		sum += uint64(binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	if len(data) > 0 {
		sum += uint64(data[0]) << 8
	}
	return sum
}

func packetChecksum(data []byte, sum uint64) uint16 {
	sum = packetSum(data, sum)
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
