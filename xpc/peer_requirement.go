package xpc

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation
# include "xpc_darwin.h"
*/
import "C"
import (
	"unsafe"
)

// PeerRequirement represents an [xpc_peer_requirement_t]. (macOS 26.0+)
//
// [xpc_peer_requirement_t]: https://developer.apple.com/documentation/xpc/xpc_peer_requirement_t?language=objc
type PeerRequirement struct {
	*XpcObject
}

var _ Object = &PeerRequirement{}

// NewPeerRequirementLwcr creates a [PeerRequirement] from a LWCR object *[Dictionary]. (macOS 26.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_peer_requirement_create_lwcr
//   - https://developer.apple.com/documentation/security/defining-launch-environment-and-library-constraints?language=objc
func NewPeerRequirementLwcr(lwcr *Dictionary) (*PeerRequirement, error) {
	if err := macOSAvailable(26); err != nil {
		return nil, err
	}

	var err_out unsafe.Pointer
	ptr := C.xpcPeerRequirementCreateLwcr(lwcr.Raw(), &err_out)
	if err_out != nil {
		return nil, newRichError(err_out)
	}
	return ReleaseOnCleanup(&PeerRequirement{XpcObject: &XpcObject{ptr}}), nil
}

// NewPeerRequirementLwcrWithEntries creates a [PeerRequirement] from a LWCR object *[Dictionary] constructed
// with the given [DictionaryEntry]s. (macOS 26.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_peer_requirement_create_lwcr
//   - https://developer.apple.com/documentation/security/defining-launch-environment-and-library-constraints?language=objc
func NewPeerRequirementLwcrWithEntries(entries ...DictionaryEntry) (*PeerRequirement, error) {
	return NewPeerRequirementLwcr(NewDictionary(entries...))
}

// inactiveListenerSet configures the given [Listener] with the [PeerRequirement]. (macOS 26.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_listener_set_peer_requirement
//
// This method implements the [ListenerOption] interface.
func (pr *PeerRequirement) inactiveListenerSet(listener *Listener) {
	C.xpcListenerSetPeerRequirement(listener.Raw(), pr.Raw())
}

// inactiveSessionSet configures the given [Session] with the [PeerRequirement]. (macOS 26.0+)
//   - https://developer.apple.com/documentation/xpc/xpc_session_set_peer_requirement
//
// This method implements the [SessionOption] interface.
func (pr *PeerRequirement) inactiveSessionSet(session *Session) {
	C.xpcSessionSetPeerRequirement(session.Raw(), pr.Raw())
}
