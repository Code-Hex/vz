package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation
# include "xpc_darwin.h"
*/
import "C"
import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// Listener represents an XPC listener. (macOS 14.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_t?language=objc
type Listener struct {
	*xpcObject
	sessionHandler *cgoHandler
}

// SessionHandler is a function that handles incoming sessions.
type SessionHandler func(session *Session)

// Option represents an option for creating a [Listener].
type ListenerOption interface {
	inactiveListenerSet(*Listener)
}

var (
	_ ListenerOption = (*PeerRequirement)(nil)
)

// NewListener creates a new [Listener] for the given service name. (macOS 14.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_create
//
// You need to call [Listener.Activate] to start accepting incoming connections.
func NewListener(service string, handler SessionHandler, options ...ListenerOption) (*Listener, error) {
	if err := macOSAvailable(14); err != nil {
		return nil, err
	}

	cname := C.CString(service)
	defer C.free(unsafe.Pointer(cname))
	// Use a serial dispatch queue for the listener,
	// because the vmnet framework API does not seem to work well with concurrent queues.
	// For example, vmnet_network_create fails when using a concurrent queue.
	q := C.dispatchQueueCreateSerial(cname)
	defer C.dispatchRelease(q)
	cgoHandler, p := newCgoHandler(handler)
	var err_out unsafe.Pointer
	ptr := C.xpcListenerCreate(cname, q, C.XPC_LISTENER_CREATE_INACTIVE, p, &err_out)
	if err_out != nil {
		return nil, newRichError(err_out)
	}
	listener := ReleaseOnCleanup(&Listener{
		xpcObject:      newXpcObject(ptr),
		sessionHandler: cgoHandler,
	})
	for _, opt := range options {
		opt.inactiveListenerSet(listener)
	}
	return listener, nil
}

// callSessionHandler is called from C to handle incoming sessions.
//
//export callSessionHandler
func callSessionHandler(cgoSessionHandler, cgoSession uintptr) {
	handler := unwrapHandler[SessionHandler](cgoSessionHandler)
	session := unwrapObject[*Session](cgoSession)
	handler(session)
}

// String returns a description of the [Listener]. (macOS 14.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_copy_description
func (l *Listener) String() string {
	desc := C.xpcListenerCopyDescription(objc.Ptr(l))
	defer C.free(unsafe.Pointer(desc))
	return C.GoString(desc)
}

// Activate starts the [Listener] to accept incoming connections. (macOS 14.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_activate
func (l *Listener) Activate() error {
	var err_out unsafe.Pointer
	C.xpcListenerActivate(objc.Ptr(l), &err_out)
	if err_out != nil {
		return newRichError(err_out)
	}
	return nil
}

// Close stops the [Listener] from accepting incoming connections. (macOS 14.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_cancel
func (l *Listener) Close() error {
	C.xpcListenerCancel(objc.Ptr(l))
	return nil
}
