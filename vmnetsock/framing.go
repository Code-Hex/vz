package vmnetsock

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"golang.org/x/sys/unix"
)

func readStreamFrame(reader io.Reader, frame []byte) (int, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, err
	}

	size := uint64(binary.BigEndian.Uint32(header[:]))
	if size == 0 || size > uint64(len(frame)) {
		return 0, fmt.Errorf("invalid stream frame length %d, maximum %d", size, len(frame))
	}

	_, err := io.ReadFull(reader, frame[:int(size)])
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return int(size), err
}

func writeStreamFrames(writer io.Writer, frames [][]byte, headers [][4]byte, vectors net.Buffers) error {
	vectors = vectors[:2*len(frames)]
	var expected int64
	for j, frame := range frames {
		binary.BigEndian.PutUint32(headers[j][:], uint32(len(frame)))
		vectors[2*j] = headers[j][:]
		vectors[2*j+1] = frame
		expected += int64(4 + len(frame))
	}
	n, err := vectors.WriteTo(writer)
	if err == nil && n != expected {
		return io.ErrShortWrite
	}
	return err
}

func readDatagramFrame(conn *net.UnixConn, frame []byte) (int, error) {
	n, _, flags, _, err := conn.ReadMsgUnix(frame, nil)
	if err != nil {
		return 0, err
	}
	if flags&unix.MSG_TRUNC != 0 {
		return 0, fmt.Errorf("datagram frame exceeds maximum %d", len(frame))
	}
	if n == 0 {
		return 0, fmt.Errorf("empty datagram frame")
	}
	return n, nil
}

func writeDatagramFrame(conn *net.UnixConn, frame []byte) error {
	n, err := conn.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}
