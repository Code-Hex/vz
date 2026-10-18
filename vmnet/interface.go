package vmnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"runtime/cgo"
	"strings"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/ebitengine/purego"
)

type interfaceConfiguration struct {
	disableMACAllocation  bool
	enableChecksumOffload bool
	enableTSO             bool
	enableIsolation       bool
}

// InterfaceOption configures an interface before it starts.
// Options are applied in order, so the last value for a capability takes precedence.
type InterfaceOption func(*interfaceConfiguration)

// WithInterfaceMACAllocation controls whether vmnet generates a MAC address.
// MAC allocation is enabled by default.
func WithInterfaceMACAllocation(enabled bool) InterfaceOption {
	return func(c *interfaceConfiguration) {
		c.disableMACAllocation = !enabled
	}
}

// WithInterfaceChecksumOffload controls IPv4 header, TCP, and UDP checksum offload.
// Checksum offload is disabled by default.
func WithInterfaceChecksumOffload(enabled bool) InterfaceOption {
	return func(c *interfaceConfiguration) {
		c.enableChecksumOffload = enabled
	}
}

// WithInterfaceTSO controls TCP segmentation offload, which is disabled by default.
// When enabled, the peer must support large TCP frames.
func WithInterfaceTSO(enabled bool) InterfaceOption {
	return func(c *interfaceConfiguration) {
		c.enableTSO = enabled
	}
}

// WithInterfaceIsolation controls communication with other vmnet interfaces.
// On a Network, isolation applies to peers that also enable isolation.
// A bridged interface with isolation enabled cannot communicate with other vmnet interfaces.
// Isolation is disabled by default.
func WithInterfaceIsolation(enabled bool) InterfaceOption {
	return func(c *interfaceConfiguration) {
		c.enableIsolation = enabled
	}
}

// Interface exchanges Ethernet frames on a Network or a bridged physical interface.
// Copies share one lifetime.
// Call Stop when finished. Its zero value is not usable.
type Interface struct{ owner *interfaceOwner }
type interfaceOwner struct{ resource *interfaceResource }
type interfaceResource struct {
	mu         sync.Mutex
	session    unsafe.Pointer
	handle     cgo.Handle
	wake, done chan struct{}
	stopping   bool
	stopErr    error
	maxSize    int
	mac        [6]byte
	readers    chan struct{}
}

var interfaceWake = purego.NewCallback(func(handle uint64) {
	wake := cgo.Handle(handle).Value().(chan struct{})
	select {
	case wake <- struct{}{}:
	default:
	}
})

// StartInterfaceWithNetwork starts an interface without consuming the Network.
// It requires macOS 26 and newer. The returned interface must be stopped.
// On error it returns nil after attempting cleanup and includes any cleanup failure.
func StartInterfaceWithNetwork(n *Network, opts ...InterfaceOption) (*Interface, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if n == nil || objc.Ptr((*objc.Pointer)(n)) == nil {
		return nil, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	return startInterface(n, "", opts...)
}

// StartBridgedInterface starts an interface bridged to the named physical interface.
// BridgedInterfaces lists eligible names. It requires macOS 26 and newer.
// The returned interface must be stopped. On error it returns nil after attempting
// cleanup and includes any cleanup failure.
func StartBridgedInterface(name string, opts ...InterfaceOption) (*Interface, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if name == "" || strings.ContainsRune(name, 0) {
		return nil, fmt.Errorf("vmnet interface name must be nonempty and contain no NUL bytes")
	}
	return startInterface(nil, name, opts...)
}

// BridgedInterfaces returns the physical interface names eligible for bridged mode.
// It requires macOS 26 and newer. An empty slice means no eligible interfaces.
func BridgedInterfaces() ([]string, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	var errorPtr unsafe.Pointer
	names := vzbridge.CopyVmnetBridgedInterfaces(&errorPtr)
	if err := vmnetError(errorPtr); err != nil {
		return nil, err
	}
	defer objc.Release(names)
	result := make([]string, int(vzbridge.NSArray_Count(names)))
	for i := range result {
		result[i] = objcutil.String(vzbridge.NSArray_ObjectAtIndex(names, uint64(i)))
	}
	return result, nil
}

func startInterface(n *Network, name string, opts ...InterfaceOption) (*Interface, error) {
	var c interfaceConfiguration
	for _, opt := range opts {
		opt(&c)
	}
	r := &interfaceResource{wake: make(chan struct{}, 1), done: make(chan struct{}), readers: make(chan struct{}, 1)}
	r.handle = cgo.NewHandle(r.wake)
	var size uint64
	var errorPtr unsafe.Pointer
	var pinned runtime.Pinner
	pinned.Pin(&size)
	pinned.Pin(&r.mac[0])
	defer pinned.Unpin()
	r.session = vzbridge.StartVmnetInterface(
		(*objc.Pointer)(n), name, c.nativeFlags(),
		uint64(interfaceWake), uint64(r.handle),
		unsafe.Pointer(&size), unsafe.Pointer(&r.mac[0]), &errorPtr,
	)
	runtime.KeepAlive(n)
	return completeInterfaceStart(r, size, vmnetError(errorPtr))
}

func (c interfaceConfiguration) nativeFlags() uint32 {
	var flags uint32
	if c.disableMACAllocation {
		flags |= 1
	}
	if c.enableChecksumOffload {
		flags |= 2
	}
	if c.enableTSO {
		flags |= 4
	}
	if c.enableIsolation {
		flags |= 8
	}
	return flags
}

func completeInterfaceStart(r *interfaceResource, size uint64, startErr error) (*Interface, error) {
	if r.session == nil {
		r.handle.Delete()
		if startErr == nil {
			startErr = fmt.Errorf("vmnet returned no interface")
		}
		return nil, startErr
	}
	if startErr == nil && (size == 0 || size > uint64(^uint32(0)>>1)) {
		startErr = fmt.Errorf("vmnet returned invalid maximum packet size %d", size)
	}
	if startErr != nil {
		return nil, errors.Join(startErr, r.stop())
	}
	r.maxSize = int(size)
	owner := &interfaceOwner{resource: r}
	runtime.AddCleanup(owner, func(resource *interfaceResource) { _ = resource.stop() }, r)
	return &Interface{owner: owner}, nil
}

// MACAddress returns a copy of the address reported by vmnet, or nil if none was reported.
func (i *Interface) MACAddress() net.HardwareAddr {
	if i == nil || i.owner == nil || i.owner.resource.mac == [6]byte{} {
		return nil
	}
	return append(net.HardwareAddr(nil), i.owner.resource.mac[:]...)
}

// MaxPacketSize returns the maximum Ethernet frame size, including Ethernet headers.
func (i *Interface) MaxPacketSize() int {
	if i == nil || i.owner == nil {
		return 0
	}
	return i.owner.resource.maxSize
}

// ReadPacket waits for one frame. The buffer must hold MaxPacketSize bytes.
// A canceled context or Stop interrupts the wait. Concurrent readers share packets.
func (i *Interface) ReadPacket(ctx context.Context, frame []byte) (int, error) {
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
	if len(frame) < r.maxSize {
		return 0, io.ErrShortBuffer
	}
	select {
	case r.readers <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-r.done:
		return 0, net.ErrClosed
	}
	defer func() { <-r.readers }()
	var pinned runtime.Pinner
	pinned.Pin(&frame[0])
	defer pinned.Unpin()
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		r.mu.Lock()
		if r.stopping {
			r.mu.Unlock()
			return 0, net.ErrClosed
		}
		var errorPtr unsafe.Pointer
		n := int(vzbridge.ReadVmnetPacket(r.session, unsafe.Pointer(&frame[0]), uint64(len(frame)), &errorPtr))
		err := vmnetError(errorPtr)
		r.mu.Unlock()
		if err != nil {
			return 0, err
		}
		if n > 0 {
			if n > len(frame) {
				return 0, fmt.Errorf("vmnet returned oversized frame %d", n)
			}
			return n, nil
		}
		select {
		case <-r.wake:
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-r.done:
			return 0, net.ErrClosed
		}
	}
}

// WritePacket synchronously sends a frame and does not retain its buffer.
func (i *Interface) WritePacket(frame []byte) error {
	if i == nil || i.owner == nil {
		return fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	defer runtime.KeepAlive(i)
	r := i.owner.resource
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return net.ErrClosed
	}
	if len(frame) == 0 || len(frame) > r.maxSize {
		return fmt.Errorf("invalid vmnet frame length %d, maximum %d", len(frame), r.maxSize)
	}
	var pinned runtime.Pinner
	pinned.Pin(&frame[0])
	defer pinned.Unpin()
	var errorPtr unsafe.Pointer
	vzbridge.WriteVmnetPacket(r.session, unsafe.Pointer(&frame[0]), uint64(len(frame)), &errorPtr)
	return vmnetError(errorPtr)
}

// Stop interrupts readers and stops the interface.
// It releases local resources even if the native stop fails.
// Repeated calls return the result of the first call without retrying.
func (i *Interface) Stop() error {
	if i == nil || i.owner == nil {
		return fmt.Errorf("vmnet interface is nil or uninitialized")
	}
	defer runtime.KeepAlive(i)
	return i.owner.resource.stop()
}

func (r *interfaceResource) stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return r.stopErr
	}
	r.stopping = true
	close(r.done)
	var errorPtr unsafe.Pointer
	vzbridge.StopVmnetInterface(r.session, &errorPtr)
	r.stopErr = vmnetError(errorPtr)
	r.session = nil
	r.handle.Delete()
	return r.stopErr
}
