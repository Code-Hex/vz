package vmnet_test

import (
	"errors"
	"net"
	"net/netip"
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetNetwork(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, mode := range []struct {
		name   string
		mode   vmnet.Mode
		prefix netip.Prefix
		want   netip.Prefix
	}{
		{"Host", vmnet.ModeHost, netip.MustParsePrefix("192.168.243.99/24"), netip.MustParsePrefix("192.168.243.0/24")},
		{"Shared", vmnet.ModeShared, netip.MustParsePrefix("192.168.244.99/24"), netip.MustParsePrefix("192.168.244.0/24")},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for _, subnet := range []struct {
				name   string
				prefix netip.Prefix
			}{{"default", netip.Prefix{}}, {"explicit", mode.prefix}} {
				t.Run(subnet.name, func(t *testing.T) {
					config, err := vmnet.NewNetworkConfiguration(mode.mode)
					if err != nil {
						t.Fatal(err)
					}
					if subnet.prefix.IsValid() {
						if err := config.SetIPv4Subnet(subnet.prefix); err != nil {
							t.Fatal(err)
						}
					}
					network, err := vmnet.NewNetwork(config)
					if err != nil {
						t.Fatal(err)
					}
					got, err := network.IPv4Subnet()
					if err != nil {
						t.Fatal(err)
					}
					if subnet.prefix.IsValid() {
						if got != mode.want {
							t.Fatalf("subnet = %v, want %v", got, mode.want)
						}
					} else if !netip.MustParsePrefix("192.168.0.0/16").Contains(got.Addr()) || got.Bits() != 24 {
						t.Fatalf("default subnet = %v, want a /24 within 192.168.0.0/16", got)
					}

				})
			}
		})
	}
}

func TestVMNetInvalidInputs(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	if _, err := vmnet.NewNetworkConfiguration(vmnet.Mode(0)); err == nil {
		t.Fatal("invalid mode accepted")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		prefix netip.Prefix
	}{{"zero", netip.Prefix{}}, {"IPv6", netip.MustParsePrefix("fd00::/64")}, {"mapped IPv6", netip.MustParsePrefix("::ffff:192.168.1.0/120")}, {"point to point", netip.MustParsePrefix("192.168.1.0/31")}, {"single host", netip.MustParsePrefix("192.168.1.0/32")}} {
		t.Run(test.name, func(t *testing.T) {
			if err := config.SetIPv4Subnet(test.prefix); err == nil {
				t.Fatal("invalid subnet accepted")
			}
		})
	}
	for _, test := range []struct {
		name string
		mac  net.HardwareAddr
		ip   netip.Addr
	}{{"short MAC", net.HardwareAddr{2}, netip.MustParseAddr("192.168.1.2")}, {"long MAC", make(net.HardwareAddr, 8), netip.MustParseAddr("192.168.1.2")}, {"multicast MAC", net.HardwareAddr{1, 0, 0, 0, 0, 1}, netip.MustParseAddr("192.168.1.2")}, {"zero IP", net.HardwareAddr{2, 0, 0, 0, 0, 1}, netip.Addr{}}, {"IPv6", net.HardwareAddr{2, 0, 0, 0, 0, 1}, netip.MustParseAddr("fd00::2")}} {
		t.Run(test.name, func(t *testing.T) {
			if err := config.AddDHCPReservation(test.mac, test.ip); err == nil {
				t.Fatal("invalid reservation accepted")
			}
		})
	}
	for _, config := range []*vmnet.NetworkConfiguration{nil, {}} {
		if _, err := vmnet.NewNetwork(config); err == nil {
			t.Fatal("empty config accepted")
		}
		if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.1.0/24")); err == nil {
			t.Fatal("empty config accepted by subnet setter")
		}
		if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 1}, netip.MustParseAddr("192.168.1.2")); err == nil {
			t.Fatal("empty config accepted by reservation setter")
		}
	}
	for _, network := range []*vmnet.Network{nil, {}} {
		if _, err := network.IPv4Subnet(); err == nil {
			t.Fatal("empty network queried")
		}
	}
}

func TestVMNetIPv6Prefix(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, test := range []struct {
		name   string
		mode   vmnet.Mode
		prefix netip.Prefix
		want   netip.Prefix
	}{
		{name: "Host default", mode: vmnet.ModeHost},
		{name: "Shared default", mode: vmnet.ModeShared},
		{name: "automatic", mode: vmnet.ModeHost, prefix: netip.MustParsePrefix("::/0")},
		{name: "Host explicit", mode: vmnet.ModeHost, prefix: netip.MustParsePrefix("fd42:1234:1::99/64"), want: netip.MustParsePrefix("fd42:1234:1::/64")},
		{name: "Shared explicit", mode: vmnet.ModeShared, prefix: netip.MustParsePrefix("fd42:1234:2::99/64"), want: netip.MustParsePrefix("fd42:1234:2::/64")},
		{name: "48 bits", mode: vmnet.ModeHost, prefix: netip.MustParsePrefix("fd42:1234:3:abcd::99/48"), want: netip.MustParsePrefix("fd42:1234:3::/48")},
		{name: "128 bits", mode: vmnet.ModeHost, prefix: netip.MustParsePrefix("fd42:1234:4::99/128"), want: netip.MustParsePrefix("fd42:1234:4::99/128")},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := vmnet.NewNetworkConfiguration(test.mode)
			if err != nil {
				t.Fatal(err)
			}
			if test.prefix.IsValid() {
				copied := *config
				if err := copied.SetIPv6Prefix(test.prefix); err != nil {
					t.Fatal(err)
				}
			}
			network, err := vmnet.NewNetwork(config)
			if err != nil {
				t.Fatal(err)
			}
			got, err := network.IPv6Prefix()
			if err != nil {
				t.Fatal(err)
			}
			if test.want.IsValid() {
				if got != test.want {
					t.Fatalf("IPv6 prefix = %v, want %v", got, test.want)
				}
			} else if !got.IsValid() || !got.Addr().Is6() || !netip.MustParsePrefix("fc00::/7").Contains(got.Addr()) || got.Bits() != 64 {
				t.Fatalf("default IPv6 prefix = %v, want a ULA /64", got)
			}
		})
	}
}

func TestVMNetDualStack(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.225.99/24")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 20}, netip.MustParseAddr("192.168.225.20")); err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv6Prefix(netip.MustParsePrefix("fd42:1234:5::99/64")); err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := network.IPv4Subnet(); err != nil || got != netip.MustParsePrefix("192.168.225.0/24") {
		t.Fatalf("IPv4 subnet = %v, %v, want 192.168.225.0/24", got, err)
	}
	if got, err := network.IPv6Prefix(); err != nil || got != netip.MustParsePrefix("fd42:1234:5::/64") {
		t.Fatalf("IPv6 prefix = %v, %v, want fd42:1234:5::/64", got, err)
	}
}

func TestVMNetIPv6InvalidInputs(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	want := netip.MustParsePrefix("fd42:1234:6::/64")
	if err := config.SetIPv6Prefix(want); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		prefix netip.Prefix
	}{
		{"zero", netip.Prefix{}},
		{"IPv4", netip.MustParsePrefix("192.168.1.0/24")},
		{"mapped IPv6", netip.MustParsePrefix("::ffff:192.168.1.0/120")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := config.SetIPv6Prefix(test.prefix); err == nil {
				t.Fatal("invalid IPv6 prefix accepted")
			}
		})
	}
	for _, config := range []*vmnet.NetworkConfiguration{nil, {}} {
		if err := config.SetIPv6Prefix(netip.MustParsePrefix("fd00::/64")); err == nil {
			t.Fatal("empty config accepted by IPv6 prefix setter")
		}
	}
	for _, network := range []*vmnet.Network{nil, {}} {
		if _, err := network.IPv6Prefix(); err == nil {
			t.Fatal("empty network queried for IPv6 prefix")
		}
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := network.IPv6Prefix(); err != nil || got != want {
		t.Fatalf("IPv6 prefix after rejected changes = %v, %v, want %v", got, err, want)
	}
}

func TestVMNetNetworkCollision(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.249.0/24")); err != nil {
		t.Fatal(err)
	}
	first, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(first)
	second, err := vmnet.NewNetwork(config)
	if second != nil {
		t.Fatal("overlapping network reservation succeeded")
	}
	var nativeErr *vmnet.Error
	if !errors.As(err, &nativeErr) || nativeErr.Domain != "com.apple.vmnet" || nativeErr.Code != 1001 {
		t.Fatalf("collision error = %v, want vmnet failure code 1001", err)
	}
	if nativeErr.LocalizedDescription != "vmnet_network_create failed" {
		t.Fatalf("collision description = %q, want the failing native operation", nativeErr.LocalizedDescription)
	}
}
