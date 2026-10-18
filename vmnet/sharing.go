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

// NetworkService publishes one logical network through a launchd Mach service.
// Clients obtain a reference with [ReceiveNetwork], allowing virtual machines
// in separate processes to use the same network. XPC transfers the network
// reference; VM packets flow through vmnet.
//
// The service retains the source network until [NetworkService.Close].
// Keep the service open and its process running while clients use the network.
// A received reference does not preserve the source network reservation after
// the server releases it. Call Close when clients no longer need the network.
type NetworkService struct {
	state *networkServiceState
}

type networkServiceState struct {
	mu            sync.Mutex
	ptr           unsafe.Pointer
	serialization *NetworkSerialization
}

// ShareNetwork publishes network through the launchd Mach service named name.
// It returns after creating the service and handles client requests internally.
// Clients use [ReceiveNetwork] to obtain a reference to the same logical network.
//
// The caller must register name in the MachServices dictionary of the process's
// launchd property list and load the job with launchctl. ShareNetwork does not
// register the name. XPC may report connection or registration failures
// asynchronously, so successful creation does not confirm that clients can
// reach the service.
//
// peerRequirement is an Apple code signing requirement expression checked
// against each client executable. It must be nonempty. The expression "true"
// accepts any executable and should not be used to expose a private network.
//
// The returned service retains network. Keep the service open and this process
// running while clients use the network, then call [NetworkService.Close].
//
// ShareNetwork requires macOS 26 and newer.
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

// Close stops accepting peers, closes existing XPC connections, and releases
// the service's reference to the source network. Call it only when clients no
// longer need the shared network; their references do not retain the source
// network reservation.
//
// Repeated and concurrent calls have no additional effect.
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

// ReceiveNetwork obtains a reference to the logical network published by
// [ShareNetwork] through the launchd Mach service named name.
//
// peerRequirement is a nonempty Apple code signing requirement expression
// checked against the server executable. The server independently checks the
// client's signature against the requirement passed to ShareNetwork.
//
// ctx must be non-nil. Use a deadline to bound the request, including service
// startup and authentication. After ReceiveNetwork returns successfully,
// cancellation of ctx does not affect the returned network.
//
// Pass the network to [StartInterfaceWithNetwork] or to
// vz.NewVMNetNetworkDeviceAttachment to connect a VM. XPC transfers the network
// reference, not VM packets. The sharing service and its process must remain
// running while the client uses the network.
//
// ReceiveNetwork requires macOS 26 and newer.
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
