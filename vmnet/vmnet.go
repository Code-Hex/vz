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

type pointer = objc.Pointer

// NetworkConfiguration configures a logical network shared by vmnet attachments.
// Create it with NewNetworkConfiguration. Its settings are immutable after
// construction, and its zero value is not usable.
type NetworkConfiguration struct {
	pointer
	state *configurationState
}

type configurationState struct {
	mu           sync.Mutex
	reservations map[[6]byte]netip.Addr
}

// NetworkConfigurationOption configures a network before native allocation.
// Use options such as [WithMTU] with [NewNetworkConfiguration].
type NetworkConfigurationOption func(*networkConfigurationOptions) error

type networkConfigurationOptions struct {
	mtu                         int
	ipv4Subnet                  netip.Prefix
	ipv6Prefix                  netip.Prefix
	reservations                map[[6]byte]netip.Addr
	dhcpDisabled                bool
	dnsProxyDisabled            bool
	nat44Disabled               bool
	nat66Disabled               bool
	routerAdvertisementDisabled bool
}

// WithIPv4Subnet sets the network's IPv4 subnet. Host bits are ignored.
// Prefixes longer than /30 are rejected. The network, host (second), and
// broadcast addresses are reserved. DHCP reservations are checked against the
// final subnet after all options have been applied.
func WithIPv4Subnet(prefix netip.Prefix) NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		if !prefix.IsValid() || !prefix.Addr().Is4() || prefix.Bits() > 30 {
			return fmt.Errorf("invalid vmnet IPv4 subnet %v", prefix)
		}
		options.ipv4Subnet = prefix.Masked()
		return nil
	}
}

// WithIPv6Prefix sets the network's IPv6 prefix. Host bits are ignored.
// IPv4-mapped IPv6 prefixes are rejected. The prefix ::/0 selects an automatic
// IPv6 prefix.
func WithIPv6Prefix(prefix netip.Prefix) NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		if !prefix.IsValid() || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
			return fmt.Errorf("invalid vmnet IPv6 prefix %v", prefix)
		}
		options.ipv6Prefix = prefix.Masked()
		return nil
	}
}

// WithDHCPReservation reserves an IPv4 address for a unicast Ethernet MAC
// address. It copies mac when the option is created. Repeated reservations for
// the same MAC use the last address.
//
// The address must belong to the final IPv4 subnet and cannot be its network,
// host, or broadcast address, regardless of option order. With an automatically
// selected subnet, subnet validation occurs in [NewNetwork].
func WithDHCPReservation(mac net.HardwareAddr, address netip.Addr) NetworkConfigurationOption {
	var key [6]byte
	copy(key[:], mac)
	validMAC := len(mac) == len(key) && key[0]&1 == 0
	return func(options *networkConfigurationOptions) error {
		if !validMAC {
			return fmt.Errorf("invalid vmnet unicast Ethernet address")
		}
		if err := validateDHCPReservation(address, netip.Prefix{}); err != nil {
			return err
		}
		if options.reservations == nil {
			options.reservations = make(map[[6]byte]netip.Addr)
		}
		options.reservations[key] = address
		return nil
	}
}

// WithMTU sets the network's maximum transmission unit in bytes.
// NewNetworkConfiguration rejects values outside 1280 through 65535 before
// calling vmnet. The lower bound preserves the IPv6 minimum MTU; the upper
// bound prevents exceeding the Darwin interface limit. A value within this
// range may still be rejected by vmnet. If omitted, vmnet chooses the MTU.
func WithMTU(mtu int) NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		if err := validateMTU(mtu); err != nil {
			return err
		}
		options.mtu = mtu
		return nil
	}
}

// WithDHCPDisabled disables vmnet's DHCP server.
// Guests need static IPv4 configuration or another DHCP server.
func WithDHCPDisabled() NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		options.dhcpDisabled = true
		return nil
	}
}

// WithDNSProxyDisabled requests that vmnet disable its DNS proxy.
// Configure guests to use another DNS resolver. This option does not filter DNS
// traffic or guarantee that queries to the host receive no response.
func WithDNSProxyDisabled() NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		options.dnsProxyDisabled = true
		return nil
	}
}

// WithNAT44Disabled disables IPv4 network address translation.
// Guests need another route for external IPv4 traffic.
func WithNAT44Disabled() NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		options.nat44Disabled = true
		return nil
	}
}

// WithNAT66Disabled disables IPv6 network address translation.
// Guests need another route for external IPv6 traffic.
func WithNAT66Disabled() NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		options.nat66Disabled = true
		return nil
	}
}

// WithRouterAdvertisementDisabled disables IPv6 router advertisements.
// Guests need static IPv6 addresses and routes or another advertising router.
func WithRouterAdvertisementDisabled() NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		options.routerAdvertisementDisabled = true
		return nil
	}
}

// NewNetworkConfiguration creates a configuration with DHCP and IPv6 router
// advertisements enabled, an automatically selected IPv4 subnet, and a random
// IPv6 ULA prefix, unless overridden by options. Omitted service options preserve
// vmnet defaults for the selected mode. Options are applied in order before
// native allocation.
// A nil or invalid option returns an error. Repeated options use the last
// value for each setting or MAC address, but an invalid option always returns
// an error. DHCP reservations are validated against the final IPv4 subnet.
// It requires macOS 26 and newer.
func NewNetworkConfiguration(mode Mode, opts ...NetworkConfigurationOption) (*NetworkConfiguration, error) {
	var options networkConfigurationOptions
	for _, option := range opts {
		if option == nil {
			return nil, fmt.Errorf("vmnet network configuration option is nil")
		}
		if err := option(&options); err != nil {
			return nil, err
		}
	}
	for _, reservation := range options.reservations {
		if err := validateDHCPReservation(reservation, options.ipv4Subnet); err != nil {
			return nil, err
		}
	}
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if mode != ModeHost && mode != ModeShared {
		return nil, fmt.Errorf("invalid vmnet network mode %d", mode)
	}
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetworkConfiguration(int32(mode), &errorPtr)
	if err := objcutil.NewNSError(errorPtr); err != nil {
		if ptr != nil {
			vzbridge.ReleaseCFObject(ptr)
		}
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network configuration")
	}
	configuration := &NetworkConfiguration{
		pointer: *objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject),
		state: &configurationState{
			reservations: options.reservations,
		},
	}
	if err := options.apply(configuration); err != nil {
		objc.Release(configuration)
		return nil, err
	}
	return configuration, nil
}

func (options networkConfigurationOptions) apply(configuration *NetworkConfiguration) error {
	if options.dhcpDisabled {
		vzbridge.DisableVmnetDHCP(configuration)
	}
	if options.dnsProxyDisabled {
		vzbridge.DisableVmnetDNSProxy(configuration)
	}
	if options.nat44Disabled {
		vzbridge.DisableVmnetNAT44(configuration)
	}
	if options.nat66Disabled {
		vzbridge.DisableVmnetNAT66(configuration)
	}
	if options.routerAdvertisementDisabled {
		vzbridge.DisableVmnetRouterAdvertisement(configuration)
	}
	var errorPtr unsafe.Pointer
	if options.mtu != 0 {
		vzbridge.SetVmnetMTU(configuration, uint32(options.mtu), &errorPtr)
		if err := vmnetError(errorPtr); err != nil {
			return err
		}
	}
	if options.ipv4Subnet.IsValid() {
		// vmnet needs the host address to start DHCP-backed attachments.
		address := options.ipv4Subnet.Addr().Next().As4()
		mask := binary.BigEndian.Uint32(net.CIDRMask(options.ipv4Subnet.Bits(), 32))
		vzbridge.SetVmnetIPv4Subnet(configuration, binary.BigEndian.Uint32(address[:]), mask, &errorPtr)
		if err := vmnetError(errorPtr); err != nil {
			return err
		}
	}
	if options.ipv6Prefix.IsValid() {
		address := options.ipv6Prefix.Addr().As16()
		vzbridge.SetVmnetIPv6Prefix(configuration, unsafe.Pointer(&address[0]), uint8(options.ipv6Prefix.Bits()), &errorPtr)
		runtime.KeepAlive(address)
		if err := vmnetError(errorPtr); err != nil {
			return err
		}
	}
	for mac, address := range options.reservations {
		bytes := address.As4()
		vzbridge.AddVmnetDHCPReservation(configuration, unsafe.Pointer(&mac[0]), binary.BigEndian.Uint32(bytes[:]), &errorPtr)
		runtime.KeepAlive(mac)
		if err := vmnetError(errorPtr); err != nil {
			return err
		}
	}
	return nil
}

func validateMTU(mtu int) error {
	// Darwin's IF_MAXMTU is 65535. Larger values can trigger a kernel assertion.
	if mtu < 1280 || mtu > 65535 {
		return fmt.Errorf("invalid vmnet MTU %d: must be between 1280 and 65535", mtu)
	}
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
type Network pointer

// NewNetwork reserves a logical network using config without consuming it.
// The reservation lasts while the network or its attachments remain in use.
// DHCP reservations are checked against the actual subnet before the network is returned.
// It requires macOS 26 and newer.
func NewNetwork(config *NetworkConfiguration) (*Network, error) {
	if err := macos.Available(26, vzbridge.SDKVersion); err != nil {
		return nil, err
	}
	if config == nil || objc.Ptr(config) == nil || config.state == nil {
		return nil, fmt.Errorf("vmnet network configuration is nil or uninitialized")
	}
	config.state.mu.Lock()
	defer config.state.mu.Unlock()
	var errorPtr unsafe.Pointer
	ptr := vzbridge.NewVmnetNetwork(config, &errorPtr)
	if err := objcutil.NewNSError(errorPtr); err != nil {
		if ptr != nil {
			vzbridge.ReleaseCFObject(ptr)
		}
		return nil, err
	}
	if ptr == nil {
		return nil, fmt.Errorf("vmnet returned no network")
	}
	network := (*Network)(objc.NewManagedPointer(ptr, vzbridge.ReleaseCFObject))
	if len(config.state.reservations) != 0 {
		prefix, err := network.IPv4Subnet()
		if err != nil {
			objc.Release((*objc.Pointer)(network))
			return nil, err
		}
		for _, reservation := range config.state.reservations {
			if err := validateDHCPReservation(reservation, prefix); err != nil {
				objc.Release((*objc.Pointer)(network))
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
	if n == nil || objc.Ptr((*objc.Pointer)(n)) == nil {
		return netip.Prefix{}, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	packed := vzbridge.VmnetIPv4Subnet((*objc.Pointer)(n))
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
	if n == nil || objc.Ptr((*objc.Pointer)(n)) == nil {
		return netip.Prefix{}, fmt.Errorf("vmnet network is nil or uninitialized")
	}
	var address [16]byte
	prefixBits := vzbridge.VmnetIPv6Prefix((*objc.Pointer)(n), unsafe.Pointer(&address[0]))
	runtime.KeepAlive(address)
	if prefixBits > 128 {
		return netip.Prefix{}, fmt.Errorf("vmnet returned an invalid IPv6 prefix length %d", prefixBits)
	}
	return netip.PrefixFrom(netip.AddrFrom16(address), int(prefixBits)).Masked(), nil
}
