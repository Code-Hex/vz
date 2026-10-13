// Package vmnet configures logical networks provided by Apple's vmnet framework.
package vmnet

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vmnethandle"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// Mode specifies how a vmnet network connects its virtual machines.
type Mode int

const (
	// ModeHost connects virtual machines to each other and to the host.
	ModeHost Mode = 1000
	// ModeShared also connects virtual machines to outside networks using NAT.
	ModeShared Mode = 1001
)

type nativeHandle = vmnethandle.Handle

// NetworkConfiguration configures a logical network shared by vmnet attachments.
// Create it with NewNetworkConfiguration. Its zero value is not usable.
type NetworkConfiguration struct {
	nativeHandle
	state *configurationState
}

type configurationState struct {
	mu           sync.Mutex
	subnet       netip.Prefix
	reservations map[[6]byte]netip.Addr
}

// NewNetworkConfiguration creates a configuration with DHCP and IPv6 router
// advertisements enabled, an automatically selected IPv4 subnet, and a random
// IPv6 ULA prefix. It requires macOS 26 and newer.
func NewNetworkConfiguration(mode Mode) (*NetworkConfiguration, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if mode != ModeHost && mode != ModeShared {
		return nil, fmt.Errorf("invalid vmnet network mode %d", mode)
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetworkConfiguration(int32(mode), &errorPtr)
	if err := objcutil.NewNSError(errorPtr); err != nil {
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network configuration")
	}
	return &NetworkConfiguration{
		nativeHandle: vmnethandle.New(objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject)),
		state:        &configurationState{},
	}, nil
}

// SetIPv4Subnet sets the network's IPv4 subnet. Host bits in prefix are ignored.
// The first, second (host), and last addresses are reserved. Prefixes
// longer than /30 are rejected. It requires macOS 26 and newer.
func (c *NetworkConfiguration) SetIPv4Subnet(prefix netip.Prefix) error {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return err
	}
	if c == nil || vmnethandle.Pointer(c) == nil || c.state == nil {
		return fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	if !prefix.IsValid() || !prefix.Addr().Is4() || prefix.Bits() > 30 {
		return fmt.Errorf("invalid vmnet IPv4 subnet %v", prefix)
	}
	prefix = prefix.Masked()
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	for _, reservation := range c.state.reservations {
		if err := validateDHCPReservation(reservation, prefix); err != nil {
			return err
		}
	}
	// vmnet needs the host address to start DHCP-backed attachments.
	address := prefix.Addr().Next().As4()
	mask := binary.BigEndian.Uint32(net.CIDRMask(prefix.Bits(), 32))
	var errorPtr unsafe.Pointer
	vzbridge.SetVmnetIPv4Subnet(vmnethandle.Pointer(c), binary.BigEndian.Uint32(address[:]), mask, &errorPtr)
	if err := vmnetError(errorPtr); err != nil {
		return err
	}
	c.state.subnet = prefix
	return nil
}

// SetIPv6Prefix sets the network's IPv6 prefix. Host bits in prefix are ignored.
// IPv4-mapped IPv6 prefixes are rejected. The prefix ::/0 selects an automatic
// IPv6 prefix. It requires macOS 26 and newer.
func (c *NetworkConfiguration) SetIPv6Prefix(prefix netip.Prefix) error {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return err
	}
	if c == nil || vmnethandle.Pointer(c) == nil || c.state == nil {
		return fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	if !prefix.IsValid() || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return fmt.Errorf("invalid vmnet IPv6 prefix %v", prefix)
	}
	prefix = prefix.Masked()
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	address := prefix.Addr().As16()
	var errorPtr unsafe.Pointer
	vzbridge.SetVmnetIPv6Prefix(vmnethandle.Pointer(c), unsafe.Pointer(&address[0]), uint8(prefix.Bits()), &errorPtr)
	runtime.KeepAlive(address)
	return vmnetError(errorPtr)
}

// AddDHCPReservation reserves an IPv4 address for a unicast Ethernet MAC address.
// The address must belong to the subnet and cannot be its network, host, or broadcast address.
// Configure reservations before creating the network. With an automatically selected subnet,
// subnet validation occurs in NewNetwork. It requires macOS 26 and newer.
func (c *NetworkConfiguration) AddDHCPReservation(mac net.HardwareAddr, address netip.Addr) error {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return err
	}
	if c == nil || vmnethandle.Pointer(c) == nil || c.state == nil {
		return fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	if len(mac) != 6 || mac[0]&1 != 0 {
		return fmt.Errorf("invalid vmnet unicast Ethernet address %v", mac)
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if err := validateDHCPReservation(address, c.state.subnet); err != nil {
		return err
	}
	var key [6]byte
	copy(key[:], mac)
	bytes := address.As4()
	var errorPtr unsafe.Pointer
	vzbridge.AddVmnetDHCPReservation(vmnethandle.Pointer(c), unsafe.Pointer(&key[0]), binary.BigEndian.Uint32(bytes[:]), &errorPtr)
	runtime.KeepAlive(key)
	if err := vmnetError(errorPtr); err != nil {
		return err
	}
	if c.state.reservations == nil {
		c.state.reservations = make(map[[6]byte]netip.Addr)
	}
	c.state.reservations[key] = address
	return nil
}

func validateDHCPReservation(address netip.Addr, prefix netip.Prefix) error {
	if !address.Is4() || address.IsUnspecified() || address.IsMulticast() || address == netip.AddrFrom4([4]byte{255, 255, 255, 255}) {
		return fmt.Errorf("invalid vmnet IPv4 reservation %v", address)
	}
	if !prefix.IsValid() {
		return nil
	}
	if !prefix.Contains(address) {
		return fmt.Errorf("vmnet IPv4 reservation %v is outside subnet %v", address, prefix)
	}
	first := prefix.Addr()
	last := first.As4()
	mask := net.CIDRMask(prefix.Bits(), 32)
	for i := range last {
		last[i] |= ^mask[i]
	}
	if address == first || address == first.Next() || address == netip.AddrFrom4(last) {
		return fmt.Errorf("vmnet IPv4 reservation %v is reserved by subnet %v", address, prefix)
	}
	return nil
}

func vmnetError(ptr unsafe.Pointer) error {
	if err := objcutil.NewNSError(ptr); err != nil {
		return err
	}
	return nil
}

// Network is a logical network shared by one or more vmnet attachments.
// Create it with NewNetwork. Its zero value is not usable.
type Network struct{ nativeHandle }

// NewNetwork reserves a logical network using config without consuming it.
// The reservation lasts while the network or its attachments remain in use.
// DHCP reservations are checked against the actual subnet before the network is returned.
// It requires macOS 26 and newer.
func NewNetwork(config *NetworkConfiguration) (*Network, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if config == nil || vmnethandle.Pointer(config) == nil || config.state == nil {
		return nil, fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	config.state.mu.Lock()
	defer config.state.mu.Unlock()
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetwork(vmnethandle.Pointer(config), &errorPtr)
	if err := objcutil.NewNSError(errorPtr); err != nil {
		if ptr != nil {
			vzbridge.ReleaseCFObject(ptr)
		}
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network")
	}
	network := &Network{nativeHandle: vmnethandle.New(objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject))}
	if len(config.state.reservations) != 0 {
		prefix, err := network.IPv4Subnet()
		if err != nil {
			objc.Release(vmnethandle.Pointer(network))
			return nil, err
		}
		for _, reservation := range config.state.reservations {
			if err := validateDHCPReservation(reservation, prefix); err != nil {
				objc.Release(vmnethandle.Pointer(network))
				return nil, err
			}
		}
	}
	return network, nil
}

// IPv4Subnet returns the reserved IPv4 subnet, including an automatically
// selected subnet when none was configured. It requires macOS 26 and newer.
func (n *Network) IPv4Subnet() (netip.Prefix, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return netip.Prefix{}, err
	}
	if n == nil || vmnethandle.Pointer(n) == nil {
		return netip.Prefix{}, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	packed := vzbridge.VmnetIPv4Subnet(vmnethandle.Pointer(n))
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

// IPv6Prefix returns the reserved IPv6 prefix, including an automatically
// selected prefix when none was configured. It requires macOS 26 and newer.
func (n *Network) IPv6Prefix() (netip.Prefix, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return netip.Prefix{}, err
	}
	if n == nil || vmnethandle.Pointer(n) == nil {
		return netip.Prefix{}, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	var address [16]byte
	prefixBits := vzbridge.VmnetIPv6Prefix(vmnethandle.Pointer(n), unsafe.Pointer(&address[0]))
	runtime.KeepAlive(address)
	if prefixBits > 128 {
		return netip.Prefix{}, fmt.Errorf("vmnet returned an invalid IPv6 prefix length %d", prefixBits)
	}
	return netip.PrefixFrom(netip.AddrFrom16(address), int(prefixBits)).Masked(), nil
}
