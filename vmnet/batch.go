package vmnet

import (
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

const packetBatchLimit = 32
const packetBatchBytes = 256000

// ReadPackets reads Ethernet frames into frames and returns the number read.
// It waits for at least one frame, but does not wait to fill the batch.
// A read may return fewer frames than fit, even when more frames are queued.
//
// Each buffer must have capacity for [Interface.MaxPacketSize] bytes. On success,
// ReadPackets sets the lengths of frames[:n] to the received frame lengths.
// The same buffers can be passed to the next call without resetting their lengths.
// The buffers must not overlap. The caller must not use frames or its buffers
// during the call, and ReadPackets does not retain them after it returns.
//
// Canceling ctx or calling [Interface.Stop] interrupts a blocked read, including
// a read waiting for another reader to finish. On an open interface, an empty
// batch returns (0, nil).
//
// See https://developer.apple.com/documentation/vmnet/vmnet_read(_:_:_:)?language=objc
func (i *Interface) ReadPackets(ctx context.Context, frames [][]byte) (int, error) {
	if i == nil || i.owner == nil {
		return 0, fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	defer runtime.KeepAlive(i)
	r := i.owner.resource
	select {
	case <-r.done:
		return 0, net.ErrClosed
	default:
	}
	for _, frame := range frames {
		if cap(frame) < r.maxSize {
			return 0, io.ErrShortBuffer
		}
	}
	if len(frames) == 0 {
		return 0, nil
	}
	if r.maxSize > packetBatchBytes {
		return 0, fmt.Errorf("vmnet maximum packet size %d exceeds batch byte limit %d", r.maxSize, packetBatchBytes)
	}
	select {
	case r.readers <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-r.done:
		return 0, net.ErrClosed
	}
	defer func() { <-r.readers }()
	frames = frames[:min(len(frames), packetBatchLimit, packetBatchBytes/r.maxSize)]
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		r.mu.Lock()
		if r.stopping {
			r.mu.Unlock()
			return 0, net.ErrClosed
		}
		n, err := r.transferPackets(frames, true)
		r.mu.Unlock()
		if err != nil || n > 0 {
			return n, err
		}
		// Events are wake hints, not packet counts. Drain native packets before waiting.
		select {
		case <-r.wake:
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-r.done:
			return 0, net.ErrClosed
		}
	}
}

// WritePackets sends Ethernet frames and returns the number sent.
// It checks every frame before sending any, then splits the frames into native
// batches as needed. Each frame must contain between 1 and
// min(MaxPacketSize(), 256000) bytes.
//
// On error, n counts only frames confirmed by successful native calls. The failed
// call may have sent additional frames, so retrying frames[n:] can send duplicates.
// Concurrent writers may interleave their batches.
//
// The caller must not change frames or its buffers during the call.
// WritePackets does not retain them after it returns. On an open interface,
// an empty batch returns (0, nil).
//
// See https://developer.apple.com/documentation/vmnet/vmnet_write(_:_:_:)?language=objc
func (i *Interface) WritePackets(frames [][]byte) (int, error) {
	if i == nil || i.owner == nil {
		return 0, fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	defer runtime.KeepAlive(i)
	r := i.owner.resource
	select {
	case <-r.done:
		return 0, net.ErrClosed
	default:
	}
	for _, frame := range frames {
		if len(frame) == 0 || len(frame) > min(r.maxSize, packetBatchBytes) {
			return 0, fmt.Errorf("invalid vmnet frame length %d, maximum %d", len(frame), min(r.maxSize, packetBatchBytes))
		}
	}
	sent := 0
	for sent < len(frames) {
		r.mu.Lock()
		if r.stopping {
			r.mu.Unlock()
			return sent, net.ErrClosed
		}
		n, err := r.transferPackets(frames[sent:min(len(frames), sent+packetBatchLimit)], false)
		r.mu.Unlock()
		if err != nil {
			return sent, err
		}
		if n == 0 {
			return sent, io.ErrShortWrite
		}
		sent += n
	}
	return sent, nil
}

// The resource mutex protects the session throughout this synchronous call.
func (r *interfaceResource) transferPackets(frames [][]byte, reading bool) (int, error) {
	var buffers [packetBatchLimit]unsafe.Pointer
	var sizes [packetBatchLimit]uint64
	var pinned runtime.Pinner
	defer pinned.Unpin()
	for j, frame := range frames {
		size := len(frame)
		if reading {
			size = r.maxSize
			frame = frame[:size]
		}
		buffers[j] = unsafe.Pointer(&frame[0])
		sizes[j] = uint64(size)
		pinned.Pin(buffers[j])
	}
	pinned.Pin(&buffers[0])
	pinned.Pin(&sizes[0])
	var errorPtr unsafe.Pointer
	var count int32
	if reading {
		count = vzbridge.ReadVmnetPackets(r.session, unsafe.Pointer(&buffers[0]), unsafe.Pointer(&sizes[0]), int32(len(frames)), &errorPtr)
	} else {
		count = vzbridge.WriteVmnetPackets(r.session, unsafe.Pointer(&buffers[0]), unsafe.Pointer(&sizes[0]), int32(len(frames)), &errorPtr)
	}
	if err := vmnetError(errorPtr); err != nil {
		return 0, err
	}
	n := int(count)
	if n < 0 || n > len(frames) {
		return 0, fmt.Errorf("vmnet returned invalid packet count %d", n)
	}
	if reading {
		for j := range n {
			if sizes[j] == 0 || sizes[j] > uint64(r.maxSize) {
				return 0, fmt.Errorf("vmnet returned invalid frame length %d", sizes[j])
			}
		}
		for j := range n {
			frames[j] = frames[j][:int(sizes[j])]
		}
	}
	return n, nil
}
