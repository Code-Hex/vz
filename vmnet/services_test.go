package vmnet_test

import (
	"net/netip"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetDisabledServices(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, test := range []struct {
		name    string
		options []vmnet.NetworkConfigurationOption
	}{
		{
			name: "defaults",
		},
		{
			name:    "DHCP",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithDHCPDisabled()},
		},
		{
			name:    "DNS proxy",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithDNSProxyDisabled()},
		},
		{
			name:    "NAT44",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithNAT44Disabled()},
		},
		{
			name:    "NAT66",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithNAT66Disabled()},
		},
		{
			name:    "router advertisement",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithRouterAdvertisementDisabled()},
		},
		{
			name: "all",
			options: []vmnet.NetworkConfigurationOption{
				vmnet.WithDHCPDisabled(),
				vmnet.WithDNSProxyDisabled(),
				vmnet.WithNAT44Disabled(),
				vmnet.WithNAT66Disabled(),
				vmnet.WithRouterAdvertisementDisabled(),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []vmnet.Mode{vmnet.ModeHost, vmnet.ModeShared} {
				for repeat := 0; repeat < 2; repeat++ {
					options := []vmnet.NetworkConfigurationOption{
						vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.249.0/24")),
						vmnet.WithIPv6Prefix(netip.MustParsePrefix("fd42:249::/64")),
					}
					options = append(options, test.options...)
					options = append(options, test.options...)
					config, err := vmnet.NewNetworkConfiguration(mode, options...)
					if err != nil {
						t.Fatal(err)
					}
					if config == nil {
						t.Fatal("NewNetworkConfiguration returned nil without an error")
					}
				}
			}
		})
	}
}
