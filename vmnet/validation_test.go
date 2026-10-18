package vmnet_test

import (
	"net"
	"net/netip"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetDHCPReservationValidation(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, test := range []struct {
		name    string
		prefix  string
		address string
		valid   bool
	}{
		{
			"network",
			"192.168.234.99/24",
			"192.168.234.0",
			false,
		},
		{
			"host",
			"192.168.234.99/24",
			"192.168.234.1",
			false,
		},
		{
			"broadcast",
			"192.168.234.99/24",
			"192.168.234.255",
			false,
		},
		{
			"outside",
			"192.168.234.99/24",
			"192.168.235.2",
			false,
		},
		{
			"first guest",
			"192.168.234.99/24",
			"192.168.234.2",
			true,
		},
		{
			"last guest",
			"192.168.234.99/24",
			"192.168.234.254",
			true,
		},
		{
			"small network",
			"192.168.234.1/30",
			"192.168.234.0",
			false,
		},
		{
			"small host",
			"192.168.234.1/30",
			"192.168.234.1",
			false,
		},
		{
			"small broadcast",
			"192.168.234.1/30",
			"192.168.234.3",
			false,
		},
		{
			"small guest",
			"192.168.234.1/30",
			"192.168.234.2",
			true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
				vmnet.WithIPv4Subnet(netip.MustParsePrefix(test.prefix)),
				vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr(test.address)),
			)
			if (err == nil) != test.valid {
				t.Fatalf("reservation %s in %s returned %v, want valid = %v", test.address, test.prefix, err, test.valid)
			}
		})
	}
	for _, address := range []string{"0.0.0.0", "255.255.255.255", "224.0.0.1", "fd00::2", "::ffff:192.168.234.2"} {
		t.Run(address, func(t *testing.T) {
			if _, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr(address))); err == nil {
				t.Fatalf("invalid reservation %s accepted before configuring a subnet", address)
			}
		})
	}
}

func TestVMNetReservationOptionOrder(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	prefix := vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.236.0/24"))
	for _, test := range []struct {
		name    string
		address string
		valid   bool
	}{
		{
			name:    "guest",
			address: "192.168.236.2",
			valid:   true,
		},
		{
			name:    "network",
			address: "192.168.236.0",
		},
		{
			name:    "host",
			address: "192.168.236.1",
		},
		{
			name:    "broadcast",
			address: "192.168.236.255",
		},
		{
			name:    "outside",
			address: "192.168.237.2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reservation := vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr(test.address))
			for _, options := range [][]vmnet.NetworkConfigurationOption{{prefix, reservation}, {reservation, prefix}} {
				_, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, options...)
				if (err == nil) != test.valid {
					t.Fatalf("configuration error = %v, want valid %v", err, test.valid)
				}
			}
		})
	}
}

func TestVMNetReservationDefaultSubnet(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
		vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("203.0.113.7")),
	)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err == nil || network != nil {
		t.Fatalf("network = %v, error = %v, want nil network and an error", network, err)
	}
}

func TestVMNetReservationReplacement(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 2}
	want := netip.MustParsePrefix("192.168.228.0/24")
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
		vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.228.0/23")),
		vmnet.WithDHCPReservation(mac, netip.MustParseAddr("192.168.228.255")),
		vmnet.WithIPv4Subnet(want),
		vmnet.WithDHCPReservation(mac, netip.MustParseAddr("192.168.228.10")),
	)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := network.IPv4Subnet(); err != nil || got != want {
		t.Fatalf("subnet = %v, %v, want %v", got, err, want)
	}
	runtime.KeepAlive(network)
}

func TestVMNetReservationOptionSnapshot(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 2}
	original := vmnet.WithDHCPReservation(mac, netip.MustParseAddr("192.168.230.255"))
	mac[5] = 3
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
		vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.230.0/24")),
		original,
		vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("192.168.230.10")),
	)
	if err != nil {
		t.Fatalf("MAC mutation changed the option's reservation key: %v", err)
	}
	copied := *config
	network, err := vmnet.NewNetwork(&copied)
	if err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(network)
}

func TestVMNetReservationConcurrentOptionReuse(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	options := []vmnet.NetworkConfigurationOption{
		vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.226.0/24")),
		vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("192.168.226.2")),
	}
	start := make(chan struct{})
	failures := make(chan error, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost, options...)
			failures <- err
		}()
	}
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestVMNetReservationOptionReuse(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 2}
	option := vmnet.WithDHCPReservation(mac, netip.MustParseAddr("192.168.232.2"))
	first, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
		vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.232.0/24")), option,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
		option,
		vmnet.WithDHCPReservation(mac, netip.MustParseAddr("192.168.233.2")),
		vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.233.0/24")),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		config *vmnet.NetworkConfiguration
		want   netip.Prefix
	}{
		{
			config: first,
			want:   netip.MustParsePrefix("192.168.232.0/24"),
		},
		{
			config: second,
			want:   netip.MustParsePrefix("192.168.233.0/24"),
		},
	} {
		network, err := vmnet.NewNetwork(test.config)
		if err != nil {
			t.Fatalf("reusing the option changed another configuration: %v", err)
		}
		if got, err := network.IPv4Subnet(); err != nil || got != test.want {
			t.Fatalf("subnet = %v, %v, want %v", got, err, test.want)
		}
		runtime.KeepAlive(network)
	}
}

func TestVMNetIPv6OptionReplacement(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	want := netip.MustParsePrefix("fd42:1234:6::/64")
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost,
		vmnet.WithIPv6Prefix(netip.MustParsePrefix("fd42:1234:7::/64")),
		vmnet.WithIPv6Prefix(want),
	)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := network.IPv6Prefix(); err != nil || got != want {
		t.Fatalf("IPv6 prefix = %v, %v, want %v", got, err, want)
	}
}

func TestVMNetOptionsRejectInvalidValuesBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		name    string
		options []vmnet.NetworkConfigurationOption
		message string
	}{
		{
			name:    "IPv4",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithIPv4Subnet(netip.MustParsePrefix("fd00::/64"))},
			message: "invalid vmnet IPv4 subnet",
		},
		{
			name:    "IPv6",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithIPv6Prefix(netip.MustParsePrefix("192.168.1.0/24"))},
			message: "invalid vmnet IPv6 prefix",
		},
		{
			name:    "MAC",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithDHCPReservation(net.HardwareAddr{1, 0, 0, 0, 0, 2}, netip.MustParseAddr("192.168.1.2"))},
			message: "invalid vmnet unicast Ethernet address",
		},
		{
			name:    "reservation address",
			options: []vmnet.NetworkConfigurationOption{vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("fd00::2"))},
			message: "invalid vmnet IPv4 reservation",
		},
		{
			name: "reservation before subnet",
			options: []vmnet.NetworkConfigurationOption{
				vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("192.168.1.2")),
				vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.2.0/24")),
			},
			message: "is outside subnet",
		},
		{
			name: "reservation after subnet",
			options: []vmnet.NetworkConfigurationOption{
				vmnet.WithIPv4Subnet(netip.MustParsePrefix("192.168.2.0/24")),
				vmnet.WithDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("192.168.1.2")),
			},
			message: "is outside subnet",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := vmnet.NewNetworkConfiguration(vmnet.Mode(-1), test.options...)
			if config != nil || err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("configuration = %v, error = %v, want error containing %q", config, err, test.message)
			}
		})
	}
}
