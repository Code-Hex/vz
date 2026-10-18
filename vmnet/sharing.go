package vmnet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// NetworkService shares one network through a launchd Mach service.
// Call Close when the service is no longer needed.
type NetworkService struct {
	state *networkServiceState
}

type networkServiceState struct {
	mu            sync.Mutex
	ptr           unsafe.Pointer
	serialization *NetworkSerialization
}

// ShareNetwork shares network with peers satisfying peerRequirement, an Apple
// code signing requirement expression. The service name must be registered in
// the process's launchd property list. It requires macOS 26 and newer.
func ShareNetwork(name string, network *Network, peerRequirement string) (*NetworkService, error) {
	if err := validateNetworkService(name, peerRequirement); err != nil {
		return nil, err
	}
	serialization, err := network.CopySerialization()
	if err != nil {
		return nil, err
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.ListenVmnetNetwork(name, &serialization.object, peerRequirement, &errorPtr)
	if err := vmnetError(errorPtr); err != nil {
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network service")
	}
	return &NetworkService{
		state: &networkServiceState{
			ptr:           ptr,
			serialization: serialization,
		},
	}, nil
}

// Close stops accepting peers and closes existing XPC connections.
// Keep the service open while clients use the shared network.
// Repeated calls have no effect.
func (s *NetworkService) Close() error {
	if s == nil || s.state == nil {
		return nil
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if s.state.ptr != nil {
		vzbridge.CloseVmnetNetworkService(s.state.ptr)
		s.state.ptr = nil
		s.state.serialization = nil
	}
	return nil
}

// ReceiveNetwork obtains a network from a launchd Mach service whose executable
// satisfies peerRequirement, an Apple code signing requirement expression.
// The context bounds the request, including service startup and authentication.
// It requires macOS 26 and newer.
func ReceiveNetwork(ctx context.Context, name, peerRequirement string) (*Network, error) {
	if err := validateNetworkService(name, peerRequirement); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, fmt.Errorf("vmnet network request context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var errorPtr unsafe.Pointer
	request := vzbridge.RequestVmnetNetwork(name, peerRequirement, &errorPtr)
	if err := vmnetError(errorPtr); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("vmnet returned no network request")
	}
	defer vzbridge.ReleaseVmnetNetworkRequest(request)
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		vzbridge.CancelVmnetNetworkRequest(request)
		close(canceled)
	})
	ptr := vzbridge.WaitVmnetNetworkRequest(request, &errorPtr)
	if !stop() {
		<-canceled
	}
	serialization, err := networkSerialization(ptr, errorPtr)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, err
	}
	return NewNetworkWithSerialization(serialization)
}

func validateNetworkService(name, requirement string) error {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return err
	}
	if name == "" || strings.ContainsRune(name, 0) {
		return fmt.Errorf("invalid vmnet Mach service name")
	}
	if requirement == "" || strings.ContainsRune(requirement, 0) {
		return fmt.Errorf("invalid vmnet peer code signing requirement")
	}
	return nil
}
