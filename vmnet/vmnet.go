// Package vmnet wraps the macOS 26 vmnet_network API:
// vmnet_network_configuration_create, set_ipv4_subnet,
// add_dhcp_reservation, and vmnet_network_create. It is consumed by
// vz.VmnetNetworkDeviceAttachment to construct VZ networks with
// caller-controlled subnet, DHCP reservations, etc.
//
// All APIs require macOS 26 or later. On older systems the
// constructors return an error.
package vmnet

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework CoreFoundation -framework vmnet
# include "cgo.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// Mode is the vmnet operating mode. Values mirror operating_modes_t
// in <vmnet/vmnet.h>.
type Mode int

const (
	// HostMode creates an isolated network shared between VMs on the
	// host. No outside connectivity.
	HostMode Mode = 1000
	// SharedMode adds Apple-managed NAT to the host's primary uplink.
	SharedMode Mode = 1001
	// BridgedMode puts the VM on the same L2 as a physical interface.
	// The configuration's external interface name must be set;
	// NewNetworkConfiguration does not expose that knob.
	BridgedMode Mode = 1002
)

// NetworkConfiguration wraps a vmnet_network_configuration_ref. Build
// it with the Set/Add methods, then pass to NewNetwork. The
// configuration is consumed by NewNetwork — do not reuse it.
type NetworkConfiguration struct {
	*objc.Pointer
}

// NewNetworkConfiguration creates a vmnet network configuration in
// the given mode.
//
// Requires macOS 26+.
func NewNetworkConfiguration(mode Mode) (*NetworkConfiguration, error) {
	var status C.int
	ptr := C.newNetworkConfiguration(C.int(mode), &status)
	if ptr == nil {
		return nil, fmt.Errorf("vmnet_network_configuration_create failed: status=%d", int(status))
	}
	c := &NetworkConfiguration{Pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(c, func(self *NetworkConfiguration) {
		if p := objc.Ptr(self); p != nil {
			C.releaseNetworkConfiguration(p)
		}
	})
	return c, nil
}

// SetIPv4Subnet configures the IPv4 subnet for the network.
//
// The argument is a normal netip.Prefix; the network base is
// normalized internally to the first usable host of the range
// (e.g. 192.168.200.0/24 → gateway 192.168.200.1), which is what
// vmnet's underlying API requires. Apple's docs call the parameter
// "subnet_addr" but the actual semantic — confirmed by
// round-tripping through vmnet_network_get_ipv4_subnet — is the
// gateway IP.
func (c *NetworkConfiguration) SetIPv4Subnet(subnet netip.Prefix) error {
	if !subnet.Addr().Is4() {
		return fmt.Errorf("vmnet: SetIPv4Subnet requires an IPv4 prefix, got %s", subnet)
	}
	if !subnet.IsValid() {
		return errors.New("vmnet: SetIPv4Subnet got an invalid Prefix")
	}
	// Normalize: gateway is the first host of the network.
	gateway := subnet.Masked().Addr().Next()
	mask := net.IP(net.CIDRMask(subnet.Bits(), 32)).String()

	gatewayCStr := C.CString(gateway.String())
	defer C.free(unsafe.Pointer(gatewayCStr))
	maskCStr := C.CString(mask)
	defer C.free(unsafe.Pointer(maskCStr))

	if rc := C.setIPv4Subnet(objc.Ptr(c), gatewayCStr, maskCStr); rc != 0 {
		return fmt.Errorf("vmnet_network_configuration_set_ipv4_subnet failed: status=%d", int(rc))
	}
	return nil
}

// AddDhcpReservation pins the given MAC address to the given IPv4
// reservation. vmnet's DHCP server will then serve the reservation
// IP to clients matching the MAC.
func (c *NetworkConfiguration) AddDhcpReservation(client net.HardwareAddr, reservation netip.Addr) error {
	if len(client) != 6 {
		return fmt.Errorf("vmnet: AddDhcpReservation requires a 6-byte MAC address, got %d bytes", len(client))
	}
	if !reservation.Is4() {
		return fmt.Errorf("vmnet: AddDhcpReservation requires an IPv4 reservation, got %s", reservation)
	}
	var mac [6]C.uint8_t
	for i, b := range client {
		mac[i] = C.uint8_t(b)
	}
	ipCStr := C.CString(reservation.String())
	defer C.free(unsafe.Pointer(ipCStr))

	if rc := C.addDhcpReservation(objc.Ptr(c), &mac[0], ipCStr); rc != 0 {
		return fmt.Errorf("vmnet_network_configuration_add_dhcp_reservation failed: status=%d", int(rc))
	}
	return nil
}

// Network wraps a vmnet_network_ref.
type Network struct {
	*objc.Pointer
}

// NewNetwork creates a vmnet network from the given configuration.
// The configuration is consumed: do not reuse it after this call.
//
// Requires macOS 26+.
func NewNetwork(config *NetworkConfiguration) (*Network, error) {
	var status C.int
	ptr := C.newNetwork(objc.Ptr(config), &status)
	if ptr == nil {
		return nil, fmt.Errorf("vmnet_network_create failed: status=%d", int(status))
	}
	return newNetwork(objc.NewPointer(ptr)), nil
}

// NewNetworkFromPointer wraps an existing vmnet_network_ref into a
// Network. The caller is responsible for ensuring the supplied
// pointer is +1 retained — the returned Network takes ownership of
// that retain count and will CFRelease it when garbage-collected.
// Used by vz.VmnetNetworkDeviceAttachment.Network() to materialize
// a Go wrapper around the attachment's underlying network.
func NewNetworkFromPointer(ptr *objc.Pointer) *Network {
	return newNetwork(ptr)
}

func newNetwork(ptr *objc.Pointer) *Network {
	n := &Network{Pointer: ptr}
	objc.SetFinalizer(n, func(self *Network) {
		if p := objc.Ptr(self); p != nil {
			C.releaseNetwork(p)
		}
	})
	return n
}
