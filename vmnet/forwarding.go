package vmnet

import (
	"fmt"
	"net/netip"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

type portForwardingRule struct {
	network      string
	externalPort uint16
	destination  netip.AddrPort
}

// WithPortForwarding forwards a host port to a guest address and port.
// Network must be "tcp" or "udp". The destination determines the IP family.
// Both ports must be nonzero. Unspecified, multicast, IPv4-mapped IPv6, and
// zoned addresses are rejected.
//
// Rules accumulate. For the same network, IP family, and external port, the last
// destination wins. Keep the guest address stable with [WithDHCPReservation]
// or static guest configuration. vmnet does not provide a host bind address;
// forwarded ports are not restricted to loopback.
func WithPortForwarding(network string, externalPort uint16, destination netip.AddrPort) NetworkConfigurationOption {
	return func(options *networkConfigurationOptions) error {
		if network != "tcp" && network != "udp" {
			return fmt.Errorf("invalid vmnet port forwarding network %q: must be tcp or udp", network)
		}
		if externalPort == 0 || destination.Port() == 0 {
			return fmt.Errorf("invalid vmnet port forwarding port: ports must be nonzero")
		}
		address := destination.Addr()
		if !address.IsValid() || address.IsUnspecified() || address.IsMulticast() || address.Is4In6() || address.Zone() != "" {
			return fmt.Errorf("invalid vmnet port forwarding destination %v", destination)
		}
		rule := portForwardingRule{
			network:      network,
			externalPort: externalPort,
			destination:  destination,
		}
		for i, existing := range options.portForwarding {
			if existing.network == network && existing.externalPort == externalPort && existing.destination.Addr().Is6() == address.Is6() {
				options.portForwarding[i] = rule
				return nil
			}
		}
		options.portForwarding = append(options.portForwarding, rule)
		return nil
	}
}

func (rule portForwardingRule) apply(configuration *NetworkConfiguration) error {
	address := rule.destination.Addr()
	var bytes [16]byte
	if address.Is4() {
		ipv4 := address.As4()
		copy(bytes[:], ipv4[:])
	} else {
		bytes = address.As16()
	}
	var errorPtr unsafe.Pointer
	vzbridge.AddVmnetPortForwarding(configuration, rule.network == "udp", address.Is6(), rule.externalPort, rule.destination.Port(), unsafe.Pointer(&bytes[0]), &errorPtr)
	runtime.KeepAlive(bytes)
	return vmnetError(errorPtr)
}
