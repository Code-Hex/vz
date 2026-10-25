package vz

import (
	"errors"
	"iter"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// VirtioQueue carries descriptor chains between a device and the guest.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueue?language=objc
type VirtioQueue struct {
	*pointer
}

// NextElement returns the next available descriptor chain, or nil.
// Keep calling NextElement until it returns nil to re-enable notifications for
// this queue. If a loop exits early with break or return, explicitly resume
// consuming the queue later. Waiting for another notification will not resume it.
// Return each processed element with [VirtioQueueElement.ReturnToQueue].
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueue/nextelement()?language=objc
func (q *VirtioQueue) NextElement() *VirtioQueueElement {
	native := vzbridge.VZVirtioQueue_NextElement(q)
	if native == nil {
		return nil
	}
	return &VirtioQueueElement{pointer: native, returned: new(bool)}
}

// Elements returns an iterator over the queue's available descriptor chains.
// Iteration consumes elements from the live queue. Running the iterator again
// continues with remaining or newly available elements; it does not replay them.
// Call [VirtioQueueElement.ReturnToQueue] when each element has been processed.
// The iterator does not return elements to the guest.
//
// Let the loop finish so [VirtioQueue.NextElement] can return nil and re-enable
// notifications for this queue. If the loop exits early with break or return,
// explicitly resume iteration or call NextElement until it returns nil. Do not
// wait for another notification to resume consuming the queue.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueue/nextelement()?language=objc
func (q *VirtioQueue) Elements() iter.Seq[*VirtioQueueElement] {
	return func(yield func(*VirtioQueueElement) bool) {
		for element := q.NextElement(); element != nil; element = q.NextElement() {
			if !yield(element) {
				return
			}
		}
	}
}

// QueueIndex returns the queue's index within its device.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueue/queueindex?language=objc
func (q *VirtioQueue) QueueIndex() uint16 {
	return vzbridge.VZVirtioQueue_QueueIndex(q)
}

// QueueSize returns the maximum number of available elements.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueue/queuesize?language=objc
func (q *VirtioQueue) QueueSize() uint16 {
	return vzbridge.VZVirtioQueue_QueueSize(q)
}

// VirtioQueueElement is a descriptor chain obtained from NextElement.
// Reads and writes consume guest buffers. Prefer consuming reads over PeekBytes
// because the guest may change its memory between consecutive reads.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement?language=objc
type VirtioQueueElement struct {
	*pointer
	returned *bool
}

var errVirtioElementReturned = errors.New("Virtio queue element has already been returned")

// ReturnToQueue returns the descriptor chain to the guest.
// Further reads, writes, and returns report an error.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/returntoqueue()?language=objc
func (e *VirtioQueueElement) ReturnToQueue() error {
	var err error
	vzbridge.OnCustomVirtioQueue(e, func() {
		if *e.returned {
			err = errVirtioElementReturned
			return
		}
		vzbridge.VZVirtioQueueElement_ReturnToQueue(e)
		*e.returned = true
	})
	return err
}

// ReadBytes copies and consumes exactly length bytes from the read buffers.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/readbytes(withexactlength:)?language=objc
func (e *VirtioQueueElement) ReadBytes(length uint64) ([]byte, error) {
	var data []byte
	var err error
	vzbridge.OnCustomVirtioQueue(e, func() {
		if *e.returned {
			err = errVirtioElementReturned
			return
		}
		var nativeError unsafe.Pointer
		native := vzbridge.VZVirtioQueueElement_ReadBytesWithExactLength_Error(e, length, &nativeError)
		data = objcutil.Bytes(native)
		if nativeError != nil {
			err = objcutil.NewNSError(nativeError)
		}
	})
	return data, err
}

// PeekBytes copies exactly length bytes without consuming the read buffers.
// The guest may change these bytes before the next read.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/peekintoreadbuffers(withexactlength:)?language=objc
func (e *VirtioQueueElement) PeekBytes(length uint64) ([]byte, error) {
	var data []byte
	var err error
	vzbridge.OnCustomVirtioQueue(e, func() {
		if *e.returned {
			err = errVirtioElementReturned
			return
		}
		var nativeError unsafe.Pointer
		native := vzbridge.VZVirtioQueueElement_PeekIntoReadBuffersWithExactLength_Error(e, length, &nativeError)
		data = objcutil.Bytes(native)
		if nativeError != nil {
			err = objcutil.NewNSError(nativeError)
		}
	})
	return data, err
}

// WriteBytes copies all of data into the write buffers and consumes those bytes.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/write(_:)?language=objc
func (e *VirtioQueueElement) WriteBytes(data []byte) error {
	var err error
	vzbridge.OnCustomVirtioQueue(e, func() {
		if *e.returned {
			err = errVirtioElementReturned
			return
		}
		var nativeError unsafe.Pointer
		vzbridge.VZVirtioQueueElement_WriteBuffer_ExactLength_Error(e, unsafe.Pointer(unsafe.SliceData(data)), uint64(len(data)), &nativeError)
		runtime.KeepAlive(data)
		if nativeError != nil {
			err = objcutil.NewNSError(nativeError)
		}
	})
	return err
}

// ReadBuffersByteCount returns the total size of the read buffers.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/readbuffersbytecount?language=objc
func (e *VirtioQueueElement) ReadBuffersByteCount() uint64 {
	return vzbridge.VZVirtioQueueElement_ReadBuffersByteCount(e)
}

// ReadBuffersAvailableByteCount returns the number of unread bytes.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/readbuffersavailablebytecount?language=objc
func (e *VirtioQueueElement) ReadBuffersAvailableByteCount() uint64 {
	return vzbridge.VZVirtioQueueElement_ReadBuffersAvailableByteCount(e)
}

// WriteBuffersByteCount returns the total size of the write buffers.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/writebuffersbytecount?language=objc
func (e *VirtioQueueElement) WriteBuffersByteCount() uint64 {
	return vzbridge.VZVirtioQueueElement_WriteBuffersByteCount(e)
}

// WriteBuffersAvailableByteCount returns the number of bytes still writable.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/writebuffersavailablebytecount?language=objc
func (e *VirtioQueueElement) WriteBuffersAvailableByteCount() uint64 {
	return vzbridge.VZVirtioQueueElement_WriteBuffersAvailableByteCount(e)
}

// WrittenByteCount returns the number of bytes written to this element.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/writtenbytecount?language=objc
func (e *VirtioQueueElement) WrittenByteCount() uint64 {
	return vzbridge.VZVirtioQueueElement_WrittenByteCount(e)
}

// ReadBytesIntoBuffer copies and consumes len(buffer) bytes from the read buffers.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/readbytes(intobuffer:exactlength:)?language=objc
func (e *VirtioQueueElement) ReadBytesIntoBuffer(buffer []byte) error {
	var err error
	vzbridge.OnCustomVirtioQueue(e, func() {
		if *e.returned {
			err = errVirtioElementReturned
			return
		}
		var nativeError unsafe.Pointer
		vzbridge.VZVirtioQueueElement_ReadBytesIntoBuffer_ExactLength_Error(e, unsafe.Pointer(unsafe.SliceData(buffer)), uint64(len(buffer)), &nativeError)
		runtime.KeepAlive(buffer)
		if nativeError != nil {
			err = objcutil.NewNSError(nativeError)
		}
	})
	return err
}

// ReadBuffers copies and consumes all remaining read buffers.
// Each returned slice owns its bytes independently of guest memory.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement/readbuffers()?language=objc
func (e *VirtioQueueElement) ReadBuffers() ([][]byte, error) {
	var buffers [][]byte
	var err error
	vzbridge.OnCustomVirtioQueue(e, func() {
		if *e.returned {
			err = errVirtioElementReturned
			return
		}
		native := nativeArray(vzbridge.VZVirtioQueueElement_ReadBuffers(e))
		buffers = make([][]byte, len(native))
		for i, data := range native {
			buffers[i] = objcutil.Bytes(data)
		}
	})
	return buffers, err
}
