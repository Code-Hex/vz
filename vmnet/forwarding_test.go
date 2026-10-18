package vmnet_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestPortForwardingValidation(t *testing.T) {
	for _, test := range []struct {
		name         string
		network      string
		externalPort uint16
		destination  string
	}{
		{
			name:         "unsupported protocol",
			network:      "sctp",
			externalPort: 8080,
			destination:  "192.168.120.10:80",
		},
		{
			name:         "uppercase protocol",
			network:      "TCP",
			externalPort: 8080,
			destination:  "192.168.120.10:80",
		},
		{
			name:        "zero external port",
			network:     "tcp",
			destination: "192.168.120.10:80",
		},
		{
			name:         "zero internal port",
			network:      "udp",
			externalPort: 8080,
			destination:  "192.168.120.10:0",
		},
		{
			name:         "invalid destination",
			network:      "tcp",
			externalPort: 8080,
		},
		{
			name:         "unspecified IPv4",
			network:      "tcp",
			externalPort: 8080,
			destination:  "0.0.0.0:80",
		},
		{
			name:         "unspecified IPv6",
			network:      "tcp",
			externalPort: 8080,
			destination:  "[::]:80",
		},
		{
			name:         "multicast IPv4",
			network:      "udp",
			externalPort: 8080,
			destination:  "224.0.0.1:80",
		},
		{
			name:         "multicast IPv6",
			network:      "udp",
			externalPort: 8080,
			destination:  "[ff02::1]:80",
		},
		{
			name:         "mapped IPv4",
			network:      "tcp",
			externalPort: 8080,
			destination:  "[::ffff:192.168.120.10]:80",
		},
		{
			name:         "IPv6 zone",
			network:      "tcp",
			externalPort: 8080,
			destination:  "[fe80::1%en0]:80",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			destination := netip.AddrPortFrom(netip.Addr{}, 80)
			if test.destination != "" {
				destination = netip.MustParseAddrPort(test.destination)
			}
			config, err := vmnet.NewNetworkConfiguration(vmnet.Mode(-1),
				vmnet.WithPortForwarding(test.network, test.externalPort, destination),
			)
			if config != nil || err == nil || !strings.Contains(err.Error(), "port forwarding") {
				t.Fatalf("NewNetworkConfiguration = %v, %v; want port forwarding validation error", config, err)
			}
		})
	}
}

func TestPortForwardingConfiguration(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	options := []vmnet.NetworkConfigurationOption{
		vmnet.WithPortForwarding("tcp", 58080, netip.MustParseAddrPort("192.168.120.10:80")),
		vmnet.WithPortForwarding("tcp", 58080, netip.MustParseAddrPort("192.168.120.11:81")),
		vmnet.WithPortForwarding("udp", 58080, netip.MustParseAddrPort("192.168.120.10:80")),
		vmnet.WithPortForwarding("tcp", 58080, netip.MustParseAddrPort("[fd42:1234:120::10]:80")),
		vmnet.WithPortForwarding("udp", 58080, netip.MustParseAddrPort("[fd42:1234:120::10]:80")),
	}
	for range 2 {
		config, err := vmnet.NewNetworkConfiguration(vmnet.ModeShared, options...)
		if err != nil || config == nil {
			t.Fatalf("NewNetworkConfiguration = %v, %v", config, err)
		}
	}
}
