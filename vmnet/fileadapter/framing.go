package fileadapter

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

func writeStreamFrame(writer io.Writer, frame []byte) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(frame)))

	for _, data := range [][]byte{header[:], frame} {
		for len(data) > 0 {
			n, err := writer.Write(data)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			data = data[n:]
		}
	}
	return nil
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
