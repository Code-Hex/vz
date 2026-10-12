package vz_test

import (
	"errors"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestVMNetNetwork(t *testing.T) {
	if vz.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, mode := range []struct {
		name   string
		mode   vz.VMNetNetworkMode
		prefix netip.Prefix
		want   netip.Prefix
	}{
		{"Host", vz.VMNetNetworkModeHost, netip.MustParsePrefix("192.168.243.99/24"), netip.MustParsePrefix("192.168.243.0/24")},
		{"Shared", vz.VMNetNetworkModeShared, netip.MustParsePrefix("192.168.244.99/24"), netip.MustParsePrefix("192.168.244.0/24")},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for _, subnet := range []struct {
				name   string
				prefix netip.Prefix
			}{{"default", netip.Prefix{}}, {"explicit", mode.prefix}} {
				t.Run(subnet.name, func(t *testing.T) {
					config, err := vz.NewVMNetNetworkConfiguration(mode.mode)
					if err != nil {
						t.Fatal(err)
					}
					if subnet.prefix.IsValid() {
						if err := config.SetIPv4Subnet(subnet.prefix); err != nil {
							t.Fatal(err)
						}
					}
					network, err := vz.NewVMNetNetwork(config)
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
					attachment, err := vz.NewVMNetNetworkDeviceAttachment(network)
					if err != nil {
						t.Fatal(err)
					}
					if got := attachment.String(); got != "VMNetNetworkDeviceAttachment" {
						t.Fatalf("String = %q", got)
					}
					retained := attachment.Network()
					if gotSubnet, err := retained.IPv4Subnet(); err != nil || gotSubnet != got {
						t.Fatalf("attachment network subnet = %v, %v, want %v", gotSubnet, err, got)
					}

				})
			}
		})
	}
}

func TestVMNetInvalidInputs(t *testing.T) {
	if vz.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	if _, err := vz.NewVMNetNetworkConfiguration(vz.VMNetNetworkMode(0)); err == nil {
		t.Fatal("invalid mode accepted")
	}
	config, err := vz.NewVMNetNetworkConfiguration(vz.VMNetNetworkModeHost)
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
	for _, config := range []*vz.VMNetNetworkConfiguration{nil, {}} {
		if _, err := vz.NewVMNetNetwork(config); err == nil {
			t.Fatal("empty config accepted")
		}
		if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.1.0/24")); err == nil {
			t.Fatal("empty config accepted by subnet setter")
		}
		if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 1}, netip.MustParseAddr("192.168.1.2")); err == nil {
			t.Fatal("empty config accepted by reservation setter")
		}
	}
	for _, network := range []*vz.VMNetNetwork{nil, {}} {
		if _, err := vz.NewVMNetNetworkDeviceAttachment(network); err == nil {
			t.Fatal("empty network accepted")
		}
		if _, err := network.IPv4Subnet(); err == nil {
			t.Fatal("empty network queried")
		}
	}
	for _, attachment := range []*vz.VMNetNetworkDeviceAttachment{nil, {}} {
		if attachment.Network() != nil {
			t.Fatal("empty attachment returned a network")
		}
	}
}

func TestVMNetDHCPReservationAndGuestCommunication(t *testing.T) {
	if vz.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vz.NewVMNetNetworkConfiguration(vz.VMNetNetworkModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.247.0/24")); err != nil {
		t.Fatal(err)
	}
	addresses := []string{"192.168.247.10", "192.168.247.11"}
	macs := []net.HardwareAddr{{2, 0, 0, 0, 0, 10}, {2, 0, 0, 0, 0, 11}}
	for i, mac := range macs {
		if err := config.AddDHCPReservation(mac, netip.MustParseAddr(addresses[i])); err != nil {
			t.Fatal(err)
		}
	}
	network, err := vz.NewVMNetNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	guests := make([]*Container, 2)
	for i := range guests {
		guests[i] = newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
			attachment, err := vz.NewVMNetNetworkDeviceAttachment(network)
			if err != nil {
				return err
			}
			device, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
			if err != nil {
				return err
			}
			mac, err := vz.NewMACAddress(macs[i])
			if err != nil {
				return err
			}
			device.SetMACAddress(mac)
			config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{device})
			return nil
		})
		guest := guests[i]
		t.Cleanup(func() {
			if err := guest.Shutdown(); err != nil {
				t.Error(err)
			}
		})
		session := guest.NewSession(t)
		output, err := session.CombinedOutput("ip -4 -o addr show dev eth0")
		session.Close()
		if err != nil {
			t.Fatalf("guest IP query: %v, %s", err, output)
		}
		if !strings.Contains(string(output), "inet "+addresses[i]+"/24 ") {
			t.Fatalf("guest %d IP = %s, want %s/24", i, output, addresses[i])
		}
	}
	for i, guest := range guests {
		session := guest.NewSession(t)
		output, err := session.CombinedOutput("ping " + addresses[1-i])
		session.Close()
		if err != nil {
			t.Fatalf("guest %d ping: %v, %s", i, err, output)
		}
		if !strings.Contains(string(output), addresses[1-i]+" is alive!") {
			t.Fatalf("guest %d ping did not confirm a reply: %s", i, output)
		}
	}
}

func TestVMNetNetworkCollision(t *testing.T) {
	if vz.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vz.NewVMNetNetworkConfiguration(vz.VMNetNetworkModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.249.0/24")); err != nil {
		t.Fatal(err)
	}
	first, err := vz.NewVMNetNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(first)
	second, err := vz.NewVMNetNetwork(config)
	if second != nil {
		t.Fatal("overlapping network reservation succeeded")
	}
	var nativeErr *vz.NSError
	if !errors.As(err, &nativeErr) || nativeErr.Domain != "com.apple.vmnet" || nativeErr.Code != 1001 {
		t.Fatalf("collision error = %v, want vmnet failure code 1001", err)
	}
	if nativeErr.LocalizedDescription != "vmnet_network_create failed" {
		t.Fatalf("collision description = %q, want the failing native operation", nativeErr.LocalizedDescription)
	}
}
