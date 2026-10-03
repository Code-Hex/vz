package vz

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// SocketDeviceConfiguration for a socket device configuration.
type SocketDeviceConfiguration interface {
	objc.NSObject

	socketDeviceConfiguration()
}

type baseSocketDeviceConfiguration struct{}

func (*baseSocketDeviceConfiguration) socketDeviceConfiguration() {}

var _ SocketDeviceConfiguration = (*VirtioSocketDeviceConfiguration)(nil)

// VirtioSocketDeviceConfiguration is a configuration of the Virtio socket device.
//
// This configuration creates a Virtio socket device for the guest which communicates with the host through the Virtio interface.
// Only one Virtio socket device can be used per virtual machine.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdeviceconfiguration?language=objc
type VirtioSocketDeviceConfiguration struct {
	*pointer

	*baseSocketDeviceConfiguration
}

// NewVirtioSocketDeviceConfiguration creates a new VirtioSocketDeviceConfiguration.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtioSocketDeviceConfiguration() (*VirtioSocketDeviceConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}
	config := &VirtioSocketDeviceConfiguration{
		pointer: vzbridge.VZVirtioSocketDeviceConfiguration_Init(),
	}
	return config, nil
}

// VirtioSocketDevice a device that manages port-based connections between the guest system and the host computer.
//
// Don’t create a VirtioSocketDevice struct directly. Instead, when you request a socket device in your configuration,
// the virtual machine creates it and you can get it via SocketDevices method.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice?language=objc
type VirtioSocketDevice struct {
	vm *VirtualMachine
	*pointer
}

// Listen creates a new VirtioSocketListener which is a struct that listens for port-based connection requests
// from the guest operating system.
//
// Be sure to close the listener by calling `VirtioSocketListener.Close` after used this one.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func (v *VirtioSocketDevice) Listen(port uint32) (*VirtioSocketListener, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}
	state := &socketAcceptState{wake: make(chan struct{}, 1)}
	handle := registerNativeCallback(func(kind uint32, first, second unsafe.Pointer, value uint64) uint64 {
		if kind == 4 {
			state.close()
			return 0
		}
		connection, err := newVirtioSocketConnection(objc.NewManagedPointer(first, vzbridge.ReleaseObject))
		if state.enqueue(connResults{connection, err}) && err == nil {
			return 1
		}
		return 0
	})
	pointer := vzbridge.NewVZVirtioSocketListener(handle)
	cleanup := &socketListenerCleanup{listener: pointer, device: v, port: port, accepts: state, handle: handle}
	owner := &socketListenerOwner{cleanup: cleanup}
	listener := &VirtioSocketListener{pointer: pointer, port: port, owner: owner}
	runtime.AddCleanup(owner, (*socketListenerCleanup).close, cleanup)
	socketListenerPorts.Lock()
	socketListenerPorts.current[cleanup.key()] = handle
	vzbridge.VZVirtioSocketDevice_SetSocketListener_ForPort(v, pointer, port)
	socketListenerPorts.Unlock()
	runtime.KeepAlive(listener)
	return listener, nil
}

// Connect Initiates a connection to the specified port of the guest operating system.
//
// This method initiates the connection asynchronously, and executes the completion handler when the results are available.
// If the guest operating system doesn’t listen for connections to the specified port, this method does nothing.
//
// For a successful connection, this method sets the sourcePort property of the resulting VZVirtioSocketConnection object to a random port number.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice/3656677-connecttoport?language=objc
func (v *VirtioSocketDevice) Connect(port uint32) (*VirtioSocketConnection, error) {
	ch := make(chan connResults, 1)
	request := registerNativeRequest(func(kind uint32, first, second unsafe.Pointer, value uint64) uint64 {
		if second != nil {
			if first != nil {
				vzbridge.ReleaseObject(first)
			}
			ch <- connResults{err: newNSError(second)}
			return 0
		}
		connection, err := newVirtioSocketConnection(objc.NewManagedPointer(first, vzbridge.ReleaseObject))
		ch <- connResults{connection, err}
		return 0
	})
	vzbridge.VZVirtioSocketDevice_connectToPort(v, port, request)
	result := <-ch
	runtime.KeepAlive(v)
	return result.conn, result.err
}

type connResults struct {
	conn *VirtioSocketConnection
	err  error
}

// VirtioSocketListener a struct that listens for port-based connection requests from the guest operating system.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketlistener?language=objc
type VirtioSocketListener struct {
	*pointer
	port  uint32
	owner *socketListenerOwner
}

type socketListenerOwner struct{ cleanup *socketListenerCleanup }

type socketListenerKey struct {
	device uintptr
	port   uint32
}

var socketListenerPorts = struct {
	sync.Mutex
	current map[socketListenerKey]uint64
}{current: make(map[socketListenerKey]uint64)}

type socketListenerCleanup struct {
	handle   uint64
	once     sync.Once
	listener *objc.Pointer
	device   *VirtioSocketDevice
	port     uint32
	accepts  *socketAcceptState
}

func (s *socketListenerCleanup) key() socketListenerKey {
	return socketListenerKey{device: uintptr(objc.Ptr(s.device)), port: s.port}
}

func (s *socketListenerCleanup) close() {
	s.once.Do(func() {
		s.accepts.close()
		socketListenerPorts.Lock()
		key := s.key()
		if socketListenerPorts.current[key] == s.handle {
			delete(socketListenerPorts.current, key)
			vzbridge.VZVirtioSocketDevice_RemoveSocketListenerForPort(s.device, s.port)
		}
		socketListenerPorts.Unlock()
		vzbridge.InvalidateVZVirtioSocketListener(s.listener)
		s.device = nil
		s.listener = nil
	})
}

type socketAcceptState struct {
	mu      sync.Mutex
	pending []connResults
	wake    chan struct{}
	closed  bool
}

func (s *socketAcceptState) enqueue(result connResults) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		if result.conn != nil {
			result.conn.Close()
		}
		return false
	}
	s.pending = append(s.pending, result)
	select {
	case s.wake <- struct{}{}:
	default:
	}
	s.mu.Unlock()
	return true
}

func (s *socketAcceptState) accept() (*VirtioSocketConnection, error) {
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, fmt.Errorf("accept failed: listener has been closed: %w", net.ErrClosed)
		}
		if len(s.pending) != 0 {
			result := s.pending[0]
			s.pending[0] = connResults{}
			s.pending = s.pending[1:]
			if len(s.pending) != 0 {
				select {
				case s.wake <- struct{}{}:
				default:
				}
			}
			s.mu.Unlock()
			return result.conn, result.err
		}
		s.mu.Unlock()
		<-s.wake
	}
}

func (s *socketAcceptState) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	pending := s.pending
	s.pending = nil
	close(s.wake)
	s.mu.Unlock()
	for _, result := range pending {
		if result.conn != nil {
			result.conn.Close()
		}
	}
}

var _ net.Listener = (*VirtioSocketListener)(nil)

// Accept implements the Accept method in the Listener interface; it waits for the next call and returns a net.Conn.
func (v *VirtioSocketListener) Accept() (net.Conn, error) {
	return v.AcceptVirtioSocketConnection()
}

// AcceptVirtioSocketConnection accepts the next incoming call and returns the new connection.
func (v *VirtioSocketListener) AcceptVirtioSocketConnection() (*VirtioSocketConnection, error) {
	defer runtime.KeepAlive(v)
	return v.owner.cleanup.accepts.accept()
}

// Close stops listening on the virtio socket.
func (v *VirtioSocketListener) Close() error {
	v.owner.cleanup.close()
	runtime.KeepAlive(v)
	return nil
}

// Addr returns the listener's network address, a *VirtioSocketListenerAddr.
func (v *VirtioSocketListener) Addr() net.Addr {
	const VMADDR_CID_HOST = 2 // copied from unix pacage
	return &VirtioSocketListenerAddr{
		CID:  VMADDR_CID_HOST,
		Port: v.port,
	}
}

// VirtioSocketListenerAddr represents a network end point address for the vsock protocol.
type VirtioSocketListenerAddr struct {
	CID  uint32
	Port uint32
}

var _ net.Addr = (*VirtioSocketListenerAddr)(nil)

// Network returns "vsock".
func (a *VirtioSocketListenerAddr) Network() string { return "vsock" }

// String returns string of "<cid>:<port>"
func (a *VirtioSocketListenerAddr) String() string { return fmt.Sprintf("%d:%d", a.CID, a.Port) }

// VirtioSocketConnection is a port-based connection between the guest operating system and the host computer.
//
// You don’t create connection objects directly. When the guest operating system initiates a connection, the virtual machine creates
// the connection object and passes it to the appropriate VirtioSocketListener struct, which forwards the object to its delegate.
//
// This is implemented net.Conn interface. This is generated from duplicated a file descriptor which is returned
// from virtualization.framework. macOS cannot connect directly to the Guest operating system using vsock. The　vsock
// connection must always be made via virtualization.framework. The diagram looks like this.
//
// ┌─────────┐                     ┌────────────────────────────┐               ┌────────────┐
// │  macOS  │<─── unix socket ───>│  virtualization.framework  │<─── vsock ───>│  Guest OS  │
// └─────────┘                     └────────────────────────────┘               └────────────┘
//
// You will notice that this is not vsock in using this library. However, all data this connection goes through to the vsock
// connection to which the Guest OS is connected.
//
// This struct does not have any pointers for objects of the Objective-C. Because the various values
// of the VZVirtioSocketConnection object handled by Objective-C are no longer needed after the conversion
// to the Go struct.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketconnection?language=objc
type VirtioSocketConnection struct {
	rawConn         net.Conn
	destinationPort uint32
	sourcePort      uint32
}

var _ net.Conn = (*VirtioSocketConnection)(nil)

func newVirtioSocketConnection(ptr *objc.Pointer) (*VirtioSocketConnection, error) {
	if ptr == nil || objc.Ptr(ptr) == nil {
		return nil, fmt.Errorf("socket completion returned no connection")
	}
	defer objc.Release(ptr)
	var nativeError unsafe.Pointer
	descriptor := vzbridge.SocketConnectionDuplicatedFileDescriptor(ptr, &nativeError)
	if nativeError != nil {
		return nil, newNSError(nativeError)
	}
	if descriptor < 0 {
		return nil, fmt.Errorf("socket connection has an invalid descriptor")
	}
	file := os.NewFile(uintptr(descriptor), "vsock")
	defer file.Close()
	connection, err := net.FileConn(file)
	if err != nil {
		return nil, err
	}
	return &VirtioSocketConnection{
		rawConn:         connection,
		destinationPort: vzbridge.VZVirtioSocketConnection_DestinationPort(ptr),
		sourcePort:      vzbridge.VZVirtioSocketConnection_SourcePort(ptr),
	}, nil
}

// Read reads data from connection of the vsock protocol.
func (v *VirtioSocketConnection) Read(b []byte) (n int, err error) { return v.rawConn.Read(b) }

// Write writes data to the connection of the vsock protocol.
func (v *VirtioSocketConnection) Write(b []byte) (n int, err error) { return v.rawConn.Write(b) }

// Close will be called when caused something error in socket.
func (v *VirtioSocketConnection) Close() error {
	return v.rawConn.Close()
}

// LocalAddr returns the local network address.
func (v *VirtioSocketConnection) LocalAddr() net.Addr { return v.rawConn.LocalAddr() }

// RemoteAddr returns the remote network address.
func (v *VirtioSocketConnection) RemoteAddr() net.Addr { return v.rawConn.RemoteAddr() }

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
func (v *VirtioSocketConnection) SetDeadline(t time.Time) error { return v.rawConn.SetDeadline(t) }

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (v *VirtioSocketConnection) SetReadDeadline(t time.Time) error {
	return v.rawConn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (v *VirtioSocketConnection) SetWriteDeadline(t time.Time) error {
	return v.rawConn.SetWriteDeadline(t)
}

// DestinationPort returns the destination port number of the connection.
func (v *VirtioSocketConnection) DestinationPort() uint32 {
	return v.destinationPort
}

// SourcePort returns the source port number of the connection.
func (v *VirtioSocketConnection) SourcePort() uint32 {
	return v.sourcePort
}
