package vz

import (
	"errors"
	"net/netip"
	"sync"
	"testing"
)

func TestVmnetAvailability(t *testing.T) {
	majorMinorVersionOnce.Do(func() {})
	previous := majorMinorVersion
	majorMinorVersion = 15
	defer func() { majorMinorVersion = previous; majorMinorVersionOnce = sync.Once{} }()
	for _, test := range []struct {
		name string
		call func() error
	}{
		{"configuration", func() error { _, err := NewVmnetNetworkConfiguration(VmnetNetworkModeHost); return err }},
		{"subnet setter", func() error { return (*VmnetNetworkConfiguration)(nil).SetIPv4Subnet(netip.Prefix{}) }},
		{"reservation", func() error { return (*VmnetNetworkConfiguration)(nil).AddDHCPReservation(nil, netip.Addr{}) }},
		{"network", func() error { _, err := NewVmnetNetwork(nil); return err }},
		{"subnet getter", func() error { _, err := (*VmnetNetwork)(nil).IPv4Subnet(); return err }},
		{"attachment", func() error { _, err := NewVmnetNetworkDeviceAttachment(nil); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrUnsupportedOSVersion) {
				t.Fatalf("error = %v, want unsupported OS", err)
			}
		})
	}
	for _, test := range []struct {
		name string
		sdk  int
		want error
	}{{"older SDK", 150000, ErrBuildTargetOSVersion}, {"26 SDK", 260000, nil}, {"newer SDK", 270000, nil}} {
		t.Run(test.name, func(t *testing.T) {
			if err := macOSBuildTargetAvailable(26, test.sdk); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
