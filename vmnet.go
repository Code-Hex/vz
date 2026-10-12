package vz

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// VMNetNetworkMode specifies how a vmnet network connects its virtual machines.
type VMNetNetworkMode int

const (
	// VMNetNetworkModeHost connects virtual machines to each other and to the host.
	VMNetNetworkModeHost VMNetNetworkMode = 1000
	// VMNetNetworkModeShared also connects virtual machines to outside networks using NAT.
	VMNetNetworkModeShared VMNetNetworkMode = 1001
)

// VMNetNetworkConfiguration configures a logical network shared by vmnet attachments.
// Create it with NewVMNetNetworkConfiguration. Its zero value is not usable.
type VMNetNetworkConfiguration struct{ pointer *objc.Pointer }

// NewVMNetNetworkConfiguration creates a configuration with DHCP enabled and an
// automatically selected IPv4 subnet. It requires macOS 26 and newer.
func NewVMNetNetworkConfiguration(mode VMNetNetworkMode) (*VMNetNetworkConfiguration, error) {
	if err := macOSAvailable(26); err != nil {
		return nil, err
	}
	if mode != VMNetNetworkModeHost && mode != VMNetNetworkModeShared {
		return nil, fmt.Errorf("invalid vmnet network mode %d", mode)
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetworkConfiguration(int32(mode), &errorPtr)
	if err := newNSError(errorPtr); err != nil {
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network configuration")
	}
	return &VMNetNetworkConfiguration{pointer: objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject)}, nil
}

// SetIPv4Subnet sets the network's IPv4 subnet. Host bits in prefix are ignored.
// The first, second (host), and last addresses are reserved. Prefixes
// longer than /30 are rejected. It requires macOS 26 and newer.
func (c *VMNetNetworkConfiguration) SetIPv4Subnet(prefix netip.Prefix) error {
	if err := macOSAvailable(26); err != nil {
		return err
	}
	if c == nil || c.pointer == nil {
		return fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	if !prefix.IsValid() || !prefix.Addr().Is4() || prefix.Bits() > 30 {
		return fmt.Errorf("invalid vmnet IPv4 subnet %v", prefix)
	}
	// vmnet needs the host address to start DHCP-backed attachments.
	address := prefix.Masked().Addr().Next().As4()
	mask := binary.BigEndian.Uint32(net.CIDRMask(prefix.Bits(), 32))
	var errorPtr unsafe.Pointer
	vzbridge.SetVmnetIPv4Subnet(c.pointer, binary.BigEndian.Uint32(address[:]), mask, &errorPtr)
	return vmnetError(errorPtr)
}

// AddDHCPReservation reserves an IPv4 address for a unicast Ethernet MAC address.
// Configure reservations before creating the network. It requires macOS 26 and newer.
func (c *VMNetNetworkConfiguration) AddDHCPReservation(mac net.HardwareAddr, address netip.Addr) error {
	if err := macOSAvailable(26); err != nil {
		return err
	}
	if c == nil || c.pointer == nil {
		return fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	if len(mac) != 6 || mac[0]&1 != 0 {
		return fmt.Errorf("invalid vmnet unicast Ethernet address %v", mac)
	}
	if !address.Is4() {
		return fmt.Errorf("invalid vmnet IPv4 reservation %v", address)
	}
	bytes := address.As4()
	var errorPtr unsafe.Pointer
	vzbridge.AddVmnetDHCPReservation(c.pointer, unsafe.Pointer(&mac[0]), binary.BigEndian.Uint32(bytes[:]), &errorPtr)
	runtime.KeepAlive(mac)
	return vmnetError(errorPtr)
}

func vmnetError(ptr unsafe.Pointer) error {
	if err := newNSError(ptr); err != nil {
		return err
	}
	return nil
}

// VMNetNetwork is a logical network shared by one or more vmnet attachments.
// Create it with NewVMNetNetwork. Its zero value is not usable.
type VMNetNetwork struct{ pointer *objc.Pointer }

// NewVMNetNetwork reserves a logical network using config without consuming it.
// The reservation lasts while the network or its attachments remain in use.
// It requires macOS 26 and newer.
func NewVMNetNetwork(config *VMNetNetworkConfiguration) (*VMNetNetwork, error) {
	if err := macOSAvailable(26); err != nil {
		return nil, err
	}
	if config == nil || config.pointer == nil {
		return nil, fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetwork(config.pointer, &errorPtr)
	if err := newNSError(errorPtr); err != nil {
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network")
	}
	return &VMNetNetwork{pointer: objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject)}, nil
}

// IPv4Subnet returns the reserved IPv4 subnet, including an automatically
// selected subnet when none was configured. It requires macOS 26 and newer.
func (n *VMNetNetwork) IPv4Subnet() (netip.Prefix, error) {
	if err := macOSAvailable(26); err != nil {
		return netip.Prefix{}, err
	}
	if n == nil || n.pointer == nil {
		return netip.Prefix{}, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	packed := vzbridge.VmnetIPv4Subnet(n.pointer)
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], uint32(packed>>32))
	var mask [4]byte
	binary.BigEndian.PutUint32(mask[:], uint32(packed))
	prefixBits, maskBits := net.IPMask(mask[:]).Size()
	if maskBits != 32 {
		return netip.Prefix{}, fmt.Errorf("vmnet returned an invalid IPv4 subnet mask")
	}
	return netip.PrefixFrom(netip.AddrFrom4(bytes), prefixBits).Masked(), nil
}
