package vmnetsock

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// Apple documents a 256 KB limit per read/write call. Use the same conservative
// decimal-KB cap as vmnet.WritePackets when grouping frames for the interface.
// See https://developer.apple.com/documentation/vmnet
const packetBatchBytes = 256000

func packetBatchSize(size int) int {
	// Match vmnet's internal 32-packet batch size, not Apple's packet limit.
	return min(32, max(1, packetBatchBytes/size))
}

type streamPacketReader struct {
	reader   *bufio.Reader
	frames   [][]byte
	consumed int
	maxSize  int
}

func newStreamPacketReader(reader io.Reader, maxSize int) *streamPacketReader {
	maxSize = min(maxSize, packetBatchBytes)
	count := packetBatchSize(maxSize)
	return &streamPacketReader{
		reader:  bufio.NewReaderSize(reader, count*(maxSize+4)),
		frames:  make([][]byte, 0, count),
		maxSize: maxSize,
	}
}

// readPackets borrows views of the reader's buffer until the next call.
// Only the first packet waits for input; the rest must already be buffered.
func (r *streamPacketReader) readPackets() ([][]byte, error) {
	_, _ = r.reader.Discard(r.consumed)
	r.consumed = 0
	r.frames = r.frames[:0]

	header, err := r.reader.Peek(4)
	if err != nil {
		if err == io.EOF && len(header) != 0 {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	size, err := streamFrameSize(header, r.maxSize)
	if err != nil {
		return nil, err
	}
	if _, err := r.reader.Peek(4 + size); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	data, _ := r.reader.Peek(r.reader.Buffered())
	for len(data) >= 4 && len(r.frames) < cap(r.frames) {
		size, err := streamFrameSize(data[:4], r.maxSize)
		if err != nil || len(data) < 4+size {
			// Leave an invalid or incomplete suffix for the next call.
			break
		}
		end := 4 + size
		r.frames = append(r.frames, data[4:end:end])
		r.consumed += end
		data = data[end:]
	}
	return r.frames, nil
}

func streamFrameSize(header []byte, maximum int) (int, error) {
	size := uint64(binary.BigEndian.Uint32(header))
	if size == 0 || size > uint64(maximum) {
		return 0, fmt.Errorf("invalid stream frame length %d, maximum %d", size, maximum)
	}
	return int(size), nil
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
