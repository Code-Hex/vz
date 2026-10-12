package vmnet_test

import (
	"net"
	"net/netip"
	"runtime"
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
		{"network", "192.168.234.99/24", "192.168.234.0", false},
		{"host", "192.168.234.99/24", "192.168.234.1", false},
		{"broadcast", "192.168.234.99/24", "192.168.234.255", false},
		{"outside", "192.168.234.99/24", "192.168.235.2", false},
		{"first guest", "192.168.234.99/24", "192.168.234.2", true},
		{"last guest", "192.168.234.99/24", "192.168.234.254", true},
		{"small network", "192.168.234.1/30", "192.168.234.0", false},
		{"small host", "192.168.234.1/30", "192.168.234.1", false},
		{"small broadcast", "192.168.234.1/30", "192.168.234.3", false},
		{"small guest", "192.168.234.1/30", "192.168.234.2", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
			if err != nil {
				t.Fatal(err)
			}
			if err := config.SetIPv4Subnet(netip.MustParsePrefix(test.prefix)); err != nil {
				t.Fatal(err)
			}
			err = config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr(test.address))
			if (err == nil) != test.valid {
				t.Fatalf("reservation %s in %s returned %v, want valid = %v", test.address, test.prefix, err, test.valid)
			}
		})
	}
	for _, address := range []string{"0.0.0.0", "255.255.255.255", "224.0.0.1", "fd00::2", "::ffff:192.168.234.2"} {
		t.Run(address, func(t *testing.T) {
			config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
			if err != nil {
				t.Fatal(err)
			}
			if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr(address)); err == nil {
				t.Fatalf("invalid reservation %s accepted before configuring a subnet", address)
			}
		})
	}
}

func TestVMNetReservationSubnetChanges(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, test := range []struct {
		name     string
		initial  string
		address  string
		rejected string
	}{
		{"network", "192.168.236.0/23", "192.168.237.0", "192.168.237.0/24"},
		{"host", "192.168.238.0/23", "192.168.239.1", "192.168.239.0/24"},
		{"broadcast", "192.168.240.0/23", "192.168.240.255", "192.168.240.0/24"},
		{"outside", "192.168.232.0/23", "192.168.233.2", "192.168.232.0/24"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
			if err != nil {
				t.Fatal(err)
			}
			if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr(test.address)); err != nil {
				t.Fatal(err)
			}
			initial := netip.MustParsePrefix(test.initial)
			if err := config.SetIPv4Subnet(initial); err != nil {
				t.Fatal(err)
			}
			if err := config.SetIPv4Subnet(netip.MustParsePrefix(test.rejected)); err == nil {
				t.Errorf("subnet %s accepted with reservation %s", test.rejected, test.address)
			}
			network, err := vmnet.NewNetwork(config)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.KeepAlive(network)
			got, err := network.IPv4Subnet()
			if err != nil || got != initial {
				t.Fatalf("subnet after rejected change = %v, %v, want %v", got, err, initial)
			}
		})
	}
}

func TestVMNetReservationConfigurationCopies(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	copied := *config
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.230.0/24")); err != nil {
		t.Fatal(err)
	}
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 2}
	if err := copied.AddDHCPReservation(mac, netip.MustParseAddr("192.168.230.1")); err == nil {
		t.Error("copied configuration accepted the host reservation")
	}
	if err := copied.AddDHCPReservation(mac, netip.MustParseAddr("192.168.230.254")); err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.230.0/25")); err == nil {
		t.Error("original configuration accepted a subnet excluding the copied configuration's reservation")
	}
}

func TestVMNetReservationDefaultSubnet(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("203.0.113.7")); err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	defer runtime.KeepAlive(network)
	if err == nil || network != nil {
		t.Fatalf("network with an out-of-subnet default reservation = %v, %v, want nil network and an error", network, err)
	}
}

func TestVMNetReservationReplacement(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.228.0/23")); err != nil {
		t.Fatal(err)
	}
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 2}
	if err := config.AddDHCPReservation(mac, netip.MustParseAddr("192.168.228.255")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddDHCPReservation(mac, netip.MustParseAddr("192.168.228.10")); err != nil {
		t.Fatal(err)
	}
	want := netip.MustParsePrefix("192.168.228.0/24")
	if err := config.SetIPv4Subnet(want); err != nil {
		t.Fatalf("subnet rejected after replacing the reservation: %v", err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(network)
	if got, err := network.IPv4Subnet(); err != nil || got != want {
		t.Fatalf("subnet after replacing the reservation = %v, %v, want %v", got, err, want)
	}
}

func TestVMNetReservationConcurrentConfiguration(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	copied := *config
	want := netip.MustParsePrefix("192.168.226.0/24")
	if err := config.SetIPv4Subnet(want); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errors := make(chan error, 16)
	var workers sync.WaitGroup
	for last := byte(2); last <= 9; last++ {
		workers.Add(2)
		go func(last byte) {
			defer workers.Done()
			<-start
			errors <- copied.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, last}, netip.AddrFrom4([4]byte{192, 168, 226, last}))
		}(last)
		go func() {
			defer workers.Done()
			<-start
			errors <- config.SetIPv4Subnet(want)
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent configuration failed: %v", err)
		}
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.226.0/30")); err == nil {
		t.Error("subnet excluding concurrently added reservations accepted")
	}
	network, err := vmnet.NewNetwork(&copied)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(network)
	if got, err := network.IPv4Subnet(); err != nil || got != want {
		t.Fatalf("subnet after concurrent configuration = %v, %v, want %v", got, err, want)
	}
}
