package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation
# include "xpc_darwin.h"
*/
import "C"
import (
	"context"
	"runtime/cgo"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// Session represents an [xpc_session_t]. (macOS 13.0+)
//
// [xpc_session_t]: https://developer.apple.com/documentation/xpc/xpc_session_t?language=objc
type Session struct {
	// Exported for use in other packages since unimplemented XPC API may require direct access to xpc_session_t.
	*xpcObject
	cancellationHandler    *cgoHandler
	incomingMessageHandler *cgoHandler
}

var _ Object = &Session{}

// SessionOption represents an option for configuring a inactive [Session].
type SessionOption interface {
	inactiveSessionSet(*Session)
}

var (
	_ SessionOption = (MessageHandler)(nil)
	_ SessionOption = (CancellationHandler)(nil)
	_ SessionOption = (*PeerRequirement)(nil)
)

// NewSession creates a new [Session] for the given Mach service name. (macOS 13.0+)
//
// [SessionOption](s) can be provided to configure the inactive session before activation.
// Available options include [MessageHandler], [CancellationHandler], and [PeerRequirement].
//   - https://developer.apple.com/documentation/xpc/xpc_session_create_mach_service
func NewSession(macServiceName string, sessionOpts ...SessionOption) (*Session, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}

	cServiceName := C.CString(macServiceName)
	defer C.free(unsafe.Pointer(cServiceName))

	var err_out unsafe.Pointer
	ptr := C.xpcSessionCreateMachService(cServiceName, nil, C.XPC_SESSION_CREATE_INACTIVE, &err_out)
	if err_out != nil {
		return nil, newRichError(err_out)
	}
	session := ReleaseOnCleanup(&Session{xpcObject: newXpcObject(ptr)})
	for _, o := range sessionOpts {
		o.inactiveSessionSet(session)
	}
	err := session.activate()
	if err != nil {
		session.Cancel()
		return nil, err
	}
	return session, nil
}

// MessageHandler is a function [SessionOption] that handles incoming messages in a [Session].
// It receives the incoming message *[Dictionary] and returns a reply message *[Dictionary].
type MessageHandler func(msg *Dictionary) (reply *Dictionary)

// inactiveSessionSet configures the given [Session] with the [MessageHandler]. (macOS 13.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_session_set_incoming_message_handler
func (mh MessageHandler) inactiveSessionSet(s *Session) {
	s.setIncomingMessageHandler(mh)
}

// CancellationHandler is a function [SessionOption] that handles session cancellation in a [Session]
// It receives the [RichError] that caused the cancellation.
type CancellationHandler func(err *RichError)

// inactiveSessionSet configures the given [Session] with the [CancellationHandler]. (macOS 13.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_session_set_cancel_handler
func (ch CancellationHandler) inactiveSessionSet(s *Session) {
	s.setCancellationHandler(ch)
}

// Reject rejects the incoming [Session] with the given reason. (macOS 14.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_reject_peer
func (s *Session) Reject(reason string) {
	cReason := C.CString(reason)
	defer C.free(unsafe.Pointer(cReason))
	C.xpcListenerRejectPeer(objc.Ptr(s), cReason)
}

// Accept creates a [SessionHandler] that accepts incoming sessions with the given [SessionOption]s.
func Accept(sessionOptions ...SessionOption) SessionHandler {
	return func(session *Session) {
		for _, opt := range sessionOptions {
			opt.inactiveSessionSet(session)
		}
	}
}

// String returns a description of the [Session]. (macOS 13.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_session_copy_description
func (s *Session) String() string {
	desc := C.xpcSessionCopyDescription(objc.Ptr(s))
	defer C.free(unsafe.Pointer(desc))
	return C.GoString(desc)
}

// activate activates the [Session]. (macOS 13.0+)
// It is called internally after applying all [SessionOption]s in [NewSession].
//   - https://developer.apple.com/documentation/xpc/xpc_session_activate
func (s *Session) activate() error {
	var err_out unsafe.Pointer
	C.xpcSessionActivate(objc.Ptr(s), &err_out)
	if err_out != nil {
		return newRichError(err_out)
	}
	return nil
}

// callMessageHandler is called from C to handle incoming messages.
//
//export callMessageHandler
func callMessageHandler(cgoMessageHandler, cgoMessage uintptr) (reply unsafe.Pointer) {
	handler := unwrapHandler[MessageHandler](cgoMessageHandler)
	message := unwrapObject[*Dictionary](cgoMessage)
	return objc.Ptr(handler(message))
}

// setIncomingMessageHandler sets the [MessageHandler] for the inactive [Session]. (macOS 13.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_session_set_incoming_message_handler
func (s *Session) setIncomingMessageHandler(handler MessageHandler) {
	cgoHandler, p := newCgoHandler(handler)
	C.xpcSessionSetIncomingMessageHandler(objc.Ptr(s), p)
	// Store the handler after setting it to avoid premature garbage collection of the previous handler.
	s.incomingMessageHandler = cgoHandler
}

// Cancel cancels the [Session]. (macOS 13.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_session_cancel
func (s *Session) Cancel() {
	C.xpcSessionCancel(objc.Ptr(s))
}

// callCancelHandler is called from C to handle session cancellation.
//
//export callCancelHandler
func callCancelHandler(cgoCancelHandler, cgoErr uintptr) {
	handler := unwrapHandler[CancellationHandler](cgoCancelHandler)
	err := unwrapObject[*RichError](cgoErr)
	handler(err)
}

// setCancellationHandler sets the [CancellationHandler] for the inactive [Session]. (macOS 13.0+)
// The handler will call [Session.handleCancellation] after executing the provided handler.
//   - https://developer.apple.com/documentation/xpc/xpc_session_set_cancel_handler
func (s *Session) setCancellationHandler(handler CancellationHandler) {
	cgoHandler, p := newCgoHandler((CancellationHandler)(func(err *RichError) {
		if handler != nil {
			handler(err)
		}
		s.handleCancellation(err)
	}))
	C.xpcSessionSetCancelHandler(objc.Ptr(s), p)
	// Store the handler after setting it to avoid premature garbage collection of the previous handler.
	s.cancellationHandler = cgoHandler
}

// handleCancellation handles [Session] cancellation by deleting the associated handles.
func (s *Session) handleCancellation(_ *RichError) {
}

type ReplyHandler func(*Dictionary, *RichError)

// callReplyHandler is called from C to handle reply messages.
//
//export callReplyHandler
func callReplyHandler(cgoReplyHandler uintptr, cgoReply, cgoError uintptr) {
	handler := unwrapHandler[ReplyHandler](cgoReplyHandler)
	reply := unwrapObject[*Dictionary](uintptr(cgoReply))
	err := unwrapObject[*RichError](cgoError)
	handler(reply, err)
}

// SendMessageWithReply sends a message *[Dictionary] to the [Session] and waits for a reply *[Dictionary]. (macOS 13.0+)
//
// Use [context.Context] to control cancellation and timeouts.
//   - https://developer.apple.com/documentation/xpc/xpc_session_send_message_with_reply_async
func (s *Session) SendMessageWithReply(ctx context.Context, message *Dictionary) (*Dictionary, error) {
	replyCh := make(chan *Dictionary, 1)
	errCh := make(chan *RichError, 1)
	replyHandler := (ReplyHandler)(func(reply *Dictionary, err *RichError) {
		defer close(replyCh)
		defer close(errCh)
		if err != nil {
			errCh <- ReleaseOnCleanup(Retain(err))
		} else {
			replyCh <- ReleaseOnCleanup(Retain(reply))
		}
	})
	cgoReplyHandler := cgo.NewHandle(replyHandler)
	defer cgoReplyHandler.Delete()
	C.xpcSessionSendMessageWithReplyAsync(objc.Ptr(s), objc.Ptr(message), C.uintptr_t(cgoReplyHandler))
	select {
	case reply := <-replyCh:
		return reply, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendDictionaryWithReply creates a message *[Dictionary] and calls [Session.SendMessageWithReply] with it. A`(macOS 13.0+)
//
// Use [context.Context] to control cancellation and timeouts.
// The message *[Dictionary] can be customized using [DictionaryEntry].
func (s *Session) SendDictionaryWithReply(ctx context.Context, entries ...DictionaryEntry) (*Dictionary, error) {
	return s.SendMessageWithReply(ctx, NewDictionary(entries...))
}
