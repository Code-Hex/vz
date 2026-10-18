package vmnet

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// NetworkSerialization is an opaque XPC representation of a logical network.
// It has no portable byte encoding. Its zero value is not usable.
type NetworkSerialization pointer

// CopySerialization copies the network's XPC representation without consuming
// the network. It requires macOS 26 and newer.
func (n *Network) CopySerialization() (*NetworkSerialization, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if n == nil || objc.Ptr((*objc.Pointer)(n)) == nil {
		return nil, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.CopyVmnetNetworkSerialization((*objc.Pointer)(n), &errorPtr)
	return networkSerialization(ptr, errorPtr)
}

func networkSerialization(ptr, errorPtr unsafe.Pointer) (*NetworkSerialization, error) {
	if err := vmnetError(errorPtr); err != nil {
		vzbridge.ReleaseXPCObject(ptr)
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network serialization")
	}
	return (*NetworkSerialization)(objc.NewManagedPointer(ptr, vzbridge.ReleaseXPCObject)), nil
}

// NewNetworkWithSerialization creates another reference to the same logical
// network without consuming the serialization. It requires macOS 26 and newer.
func NewNetworkWithSerialization(serialization *NetworkSerialization) (*Network, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if serialization == nil || objc.Ptr((*objc.Pointer)(serialization)) == nil {
		return nil, fmt.Errorf("vmnet network serialization is nil or uninitialized")
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetworkWithSerialization((*objc.Pointer)(serialization), &errorPtr)
	if err := vmnetError(errorPtr); err != nil {
		if ptr != nil {
			vzbridge.ReleaseCFObject(ptr)
		}
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network")
	}
	return (*Network)(objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject)), nil
}
