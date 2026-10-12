package vz_test

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetNetworkDeviceAttachment(t *testing.T) {
	if vz.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, test := range []struct {
		name   string
		mode   vmnet.Mode
		prefix netip.Prefix
	}{
		{"Host default", vmnet.ModeHost, netip.Prefix{}},
		{"Shared default", vmnet.ModeShared, netip.Prefix{}},
		{"Host explicit", vmnet.ModeHost, netip.MustParsePrefix("192.168.245.0/24")},
		{"Shared explicit", vmnet.ModeShared, netip.MustParsePrefix("192.168.246.0/24")},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := vmnet.NewNetworkConfiguration(test.mode)
			if err != nil {
				t.Fatal(err)
			}
			if test.prefix.IsValid() {
				if err := config.SetIPv4Subnet(test.prefix); err != nil {
					t.Fatal(err)
				}
			}
			network, err := vmnet.NewNetwork(config)
			if err != nil {
				t.Fatal(err)
			}
			want, err := network.IPv4Subnet()
			if err != nil {
				t.Fatal(err)
			}
			attachment, err := vz.NewVMNetNetworkDeviceAttachment(network)
			if err != nil {
				t.Fatal(err)
			}
			if got := attachment.String(); got != "VMNetNetworkDeviceAttachment" {
				t.Fatalf("String = %q", got)
			}
			retained := attachment.Network()
			if retained == nil {
				t.Fatal("attachment returned no network")
			}
			if got, err := retained.IPv4Subnet(); err != nil || got != want {
				t.Fatalf("attachment network subnet = %v, %v, want %v", got, err, want)
			}
		})
	}
}

func TestVMNetNetworkDeviceAttachmentInvalidInputs(t *testing.T) {
	if vz.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, network := range []*vmnet.Network{nil, {}} {
		if _, err := vz.NewVMNetNetworkDeviceAttachment(network); err == nil {
			t.Fatal("empty network accepted")
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
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
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
	network, err := vmnet.NewNetwork(config)
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
