package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import (
	"fmt"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/rs/xid"
	"golang.org/x/sys/unix"
)

// SocketDeviceConfiguration for a socket device configuration.
type SocketDeviceConfiguration interface {
	NSObject

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
	pointer

	*baseSocketDeviceConfiguration
}

// NewVirtioSocketDeviceConfiguration creates a new VirtioSocketDeviceConfiguration.
func NewVirtioSocketDeviceConfiguration() *VirtioSocketDeviceConfiguration {
	config := &VirtioSocketDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioSocketDeviceConfiguration(),
		},
	}
	runtime.SetFinalizer(config, func(self *VirtioSocketDeviceConfiguration) {
		self.Release()
	})
	return config
}

// VirtioSocketDevice a device that manages port-based connections between the guest system and the host computer.
//
// Don’t create a VirtioSocketDevice struct directly. Instead, when you request a socket device in your configuration,
// the virtual machine creates it and you can get it via SocketDevices method.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice?language=objc
type VirtioSocketDevice struct {
	id string

	dispatchQueue unsafe.Pointer
	pointer
}

var connectionHandlers = map[string]func(conn *VirtioSocketConnection, err error){}

func newVirtioSocketDevice(ptr, dispatchQueue unsafe.Pointer) *VirtioSocketDevice {
	id := xid.New().String()
	socketDevice := &VirtioSocketDevice{
		id:            id,
		dispatchQueue: dispatchQueue,
		pointer: pointer{
			ptr: ptr,
		},
	}
	connectionHandlers[id] = func(*VirtioSocketConnection, error) {}

	runtime.SetFinalizer(socketDevice, func(self *VirtioSocketDevice) {
		self.Release()
	})
	return socketDevice
}

// SetSocketListenerForPort configures an object to monitor the specified port for new connections.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice/3656679-setsocketlistener?language=objc
func (v *VirtioSocketDevice) SetSocketListenerForPort(listener *VirtioSocketListener, port uint32) {
	C.VZVirtioSocketDevice_setSocketListenerForPort(v.Ptr(), v.dispatchQueue, listener.Ptr(), C.uint32_t(port))
}

// RemoveSocketListenerForPort removes the listener object from the specfied port.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice/3656678-removesocketlistenerforport?language=objc
func (v *VirtioSocketDevice) RemoveSocketListenerForPort(listener *VirtioSocketListener, port uint32) {
	C.VZVirtioSocketDevice_removeSocketListenerForPort(v.Ptr(), v.dispatchQueue, C.uint32_t(port))
}

//export connectionHandler
func connectionHandler(connPtr, errPtr unsafe.Pointer, cid *C.char) {
	id := (*char)(cid).String()
	// see: startHandler
	conn := newVirtioSocketConnection(connPtr)
	if err := newNSError(errPtr); err != nil {
		connectionHandlers[id](conn, err)
	} else {
		connectionHandlers[id](conn, nil)
	}
}

// ConnectToPort Initiates a connection to the specified port of the guest operating system.
//
// This method initiates the connection asynchronously, and executes the completion handler when the results are available.
// If the guest operating system doesn’t listen for connections to the specifed port, this method does nothing.
//
// For a successful connection, this method sets the sourcePort property of the resulting VZVirtioSocketConnection object to a random port number.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice/3656677-connecttoport?language=objc
func (v *VirtioSocketDevice) ConnectToPort(port uint32, fn func(conn *VirtioSocketConnection, err error)) {
	connectionHandlers[v.id] = fn
	cid := charWithGoString(v.id)
	defer cid.Free()
	C.VZVirtioSocketDevice_connectToPort(v.Ptr(), v.dispatchQueue, C.uint32_t(port), cid.CString())
}

// VirtioSocketListener a struct that listens for port-based connection requests from the guest operating system.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketlistener?language=objc
type VirtioSocketListener struct {
	pointer
}

type dup struct {
	conn *VirtioSocketConnection
	err  error
}

var shouldAcceptNewConnectionHandlers = map[unsafe.Pointer]func(conn *VirtioSocketConnection) bool{}

// NewVirtioSocketListener creates a new VirtioSocketListener with connection handler.
//
// The handler is executed asynchronously. Be sure to close the connection used in the handler by calling `conn.Close`.
// This is to prevent connection leaks.
func NewVirtioSocketListener(handler func(conn *VirtioSocketConnection, err error)) *VirtioSocketListener {
	ptr := C.newVZVirtioSocketListener()
	listener := &VirtioSocketListener{
		pointer: pointer{
			ptr: ptr,
		},
	}

	dupCh := make(chan dup, 1)
	go func() {
		for dup := range dupCh {
			go handler(dup.conn, dup.err)
		}
	}()
	shouldAcceptNewConnectionHandlers[ptr] = func(conn *VirtioSocketConnection) bool {
		dupConn, err := conn.dup()
		dupCh <- dup{
			conn: dupConn,
			err:  err,
		}
		return true // must be connected
	}

	runtime.SetFinalizer(listener, func(self *VirtioSocketListener) {
		self.Release()
	})
	return listener
}

//export shouldAcceptNewConnectionHandler
func shouldAcceptNewConnectionHandler(listenerPtr, connPtr, devicePtr unsafe.Pointer) C.bool {
	_ = devicePtr // NOTO(codehex): Is this really required? How to use?

	// see: startHandler
	conn := newVirtioSocketConnection(connPtr)
	return (C.bool)(shouldAcceptNewConnectionHandlers[listenerPtr](conn))
}

// VirtioSocketConnection is a port-based connection between the guest operating system and the host computer.
//
// You don’t create connection objects directly. When the guest operating system initiates a connection, the virtual machine creates
// the connection object and passes it to the appropriate VirtioSocketListener struct, which forwards the object to its delegate.
//
// This is implemented net.Conn interface.
//
// This struct does not have any pointers for objects of the Objective-C. Because the various values
// of the VZVirtioSocketConnection object handled by Objective-C are no longer needed after the conversion
// to the Go struct.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketconnection?language=objc
type VirtioSocketConnection struct {
	id              string
	sourcePort      uint32
	destinationPort uint32
	fileDescriptor  uintptr
	file            *os.File
	laddr           net.Addr // local
	raddr           net.Addr // remote
}

var _ net.Conn = (*VirtioSocketConnection)(nil)

func newVirtioSocketConnection(ptr unsafe.Pointer) *VirtioSocketConnection {
	id := xid.New().String()
	vzVirtioSocketConnection := C.convertVZVirtioSocketConnection2Flat(ptr)
	conn := &VirtioSocketConnection{
		id:              id,
		sourcePort:      (uint32)(vzVirtioSocketConnection.sourcePort),
		destinationPort: (uint32)(vzVirtioSocketConnection.destinationPort),
		fileDescriptor:  (uintptr)(vzVirtioSocketConnection.fileDescriptor),
		file:            os.NewFile((uintptr)(vzVirtioSocketConnection.fileDescriptor), id),
		laddr: &Addr{
			CID:  unix.VMADDR_CID_HOST,
			Port: (uint32)(vzVirtioSocketConnection.destinationPort),
		},
		raddr: &Addr{
			CID:  unix.VMADDR_CID_HYPERVISOR,
			Port: (uint32)(vzVirtioSocketConnection.sourcePort),
		},
	}
	return conn
}

func (v *VirtioSocketConnection) dup() (*VirtioSocketConnection, error) {
	nfd, err := syscall.Dup(int(v.fileDescriptor))
	if err != nil {
		return nil, &net.OpError{
			Op:     "dup",
			Net:    "vsock",
			Source: v.laddr,
			Addr:   v.raddr,
			Err:    err,
		}
	}

	dupConn := new(VirtioSocketConnection)
	*dupConn = *v
	dupConn.fileDescriptor = uintptr(nfd)
	dupConn.file = os.NewFile(uintptr(nfd), v.file.Name())
	dupConn.laddr = v.laddr
	dupConn.raddr = v.raddr

	return dupConn, nil
}

// Read reads data from connection of the vsock protocol.
func (v *VirtioSocketConnection) Read(b []byte) (n int, err error) { return v.file.Read(b) }

// Write writes data to the connection of the vsock protocol.
func (v *VirtioSocketConnection) Write(b []byte) (n int, err error) { return v.file.Write(b) }

// Close will be called when caused something error in socket.
func (v *VirtioSocketConnection) Close() error {
	return v.file.Close()
}

// LocalAddr returns the local network address.
func (v *VirtioSocketConnection) LocalAddr() net.Addr { return v.laddr }

// RemoteAddr returns the remote network address.
func (v *VirtioSocketConnection) RemoteAddr() net.Addr { return v.raddr }

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
func (v *VirtioSocketConnection) SetDeadline(t time.Time) error { return v.file.SetDeadline(t) }

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (v *VirtioSocketConnection) SetReadDeadline(t time.Time) error {
	return v.file.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (v *VirtioSocketConnection) SetWriteDeadline(t time.Time) error {
	return v.file.SetWriteDeadline(t)
}

// ID returns connection ID. this ID is used as filename of the vsock protocol connection.
func (v *VirtioSocketConnection) ID() string { return v.id }

// DestinationPort returns the destination port number of the connection.
func (v *VirtioSocketConnection) DestinationPort() uint32 {
	return v.destinationPort
}

// SourcePort returns the source port number of the connection.
func (v *VirtioSocketConnection) SourcePort() uint32 {
	return v.sourcePort
}

// FileDescriptor returns the file descriptor associated with the socket.
//
// Data is sent by writing to the file descriptor.
// Data is received by reading from the file descriptor.
// A file descriptor of -1 indicates a closed connection.
func (v *VirtioSocketConnection) FileDescriptor() uintptr {
	return v.fileDescriptor
}

// Addr represents a network end point address for the vsock protocol.
type Addr struct {
	CID  uint32
	Port uint32
}

var _ net.Addr = (*Addr)(nil)

// Network returns "vsock".
func (a *Addr) Network() string { return "vsock" }

// String returns string of "<cid>:<port>"
func (a *Addr) String() string { return fmt.Sprintf("%d:%d", a.CID, a.Port) }
