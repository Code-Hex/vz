package vz_test

import (
	"log"
	"net"
	"net/netip"
	"slices"
	"testing"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/vmnet"
)

// TestVmnetSharedModeAllowsCommunicationBetweenMultipleVMs tests VmnetNetwork in SharedMode
// allows communication between multiple VMs connected to the same VmnetNetwork instance.
// This test creates two VmnetNetwork instances by serializing and deserializing the first instance,
// then boots a VM using each VmnetNetwork instance and tests communication between the two VMs.
func TestVmnetSharedModeAllowsCommunicationBetweenMultipleVMs(t *testing.T) {
	if vz.Available(26.0) {
		t.Skip("VmnetSharedMode is supported from macOS 26")
	}

	// Create VmnetNetwork instance from configuration
	config, err := vmnet.NewNetworkConfiguration(vmnet.SharedMode)
	if err != nil {
		t.Fatal(err)
	}
	network1, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	macaddress1 := randomMACAddress(t)

	// Create another VmnetNetwork instance from serialization of the first one
	serialization, err := network1.CopySerialization()
	if err != nil {
		t.Fatal(err)
	}
	network2, err := vmnet.NewNetworkWithSerialization(serialization)
	if err != nil {
		t.Fatal(err)
	}
	macaddress2 := randomMACAddress(t)

	container1 := newVirtualizationMachine(t, configureNetworkDevice(network1, macaddress1))
	container2 := newVirtualizationMachine(t, configureNetworkDevice(network2, macaddress2))
	t.Cleanup(func() {
		if err := container1.Shutdown(); err != nil {
			log.Println(err)
		}
		if err := container2.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	// Log network information
	ipv4Subnet, err := network1.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv4 subnet: %s", ipv4Subnet.String())
	prefix, err := network1.IPv6Prefix()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv6 prefix: %s", prefix.String())

	// Detect IP addresses and test communication between VMs
	container1IPv4 := container1.DetectIPv4(t, "eth0")
	t.Logf("Container 1 IPv4: %s", container1IPv4)
	container2IPv4 := container2.DetectIPv4(t, "eth0")
	t.Logf("Container 2 IPv4: %s", container2IPv4)
	container1.exec(t, "ping "+container2IPv4)
	container2.exec(t, "ping "+container1IPv4)
}

// TestVmnetSharedModeWithConfiguringIPv4 tests VmnetNetwork in SharedMode
// with custom IPv4 subnet and DHCP reservation.
// This test creates a VmnetNetwork instance with a specified IPv4 subnet and DHCP reservation,
// then boots a VM using the VmnetNetwork and verifies the VM receives the expected IP address.
func TestVmnetSharedModeWithConfiguringIPv4(t *testing.T) {
	if vz.Available(26.0) {
		t.Skip("VmnetSharedMode is supported from macOS 26")
	}
	// Create VmnetNetwork instance from configuration
	config, err := vmnet.NewNetworkConfiguration(vmnet.SharedMode)
	if err != nil {
		t.Fatal(err)
	}
	// Configure IPv4 subnet
	ipv4Subnet := detectFreeIPv4Subnet(t, netip.MustParsePrefix("192.168.5.0/24"))
	if err := config.SetIPv4Subnet(ipv4Subnet); err != nil {
		t.Fatal(err)
	}
	// Configure DHCP reservation
	macaddress := randomMACAddress(t)
	ipv4 := netip.MustParseAddr("192.168.5.15")
	if err := config.AddDhcpReservation(macaddress.HardwareAddr(), ipv4); err != nil {
		t.Fatal(err)
	}

	// Create VmnetNetwork instance
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}

	// Create VirtualizationMachine instance
	container := newVirtualizationMachine(t, configureNetworkDevice(network, macaddress))
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	// Log network information
	ipv4SubnetConfigured, err := network.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv4 subnet: %s", ipv4SubnetConfigured.String())

	// Verify the configured subnet
	// Compare with masked value to ignore host bits since Vmnet prefers to use first address as network address.
	if ipv4Subnet != ipv4SubnetConfigured.Masked() {
		t.Fatalf("expected IPv4 subnet %s, but got %s", ipv4Subnet.String(), ipv4SubnetConfigured.Masked().String())
	}

	// Log IPv6 prefix
	prefix, err := network.IPv6Prefix()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv6 prefix: %s", prefix.String())

	// Detect IP address and verify DHCP reservation
	containerIPv4 := container.DetectIPv4(t, "eth0")
	t.Logf("Container IPv4: %s", containerIPv4)
	if ipv4.String() != containerIPv4 {
		t.Fatalf("expected IPv4 %s, but got %s", ipv4, containerIPv4)
	}
}

func configureNetworkDevice(network *vmnet.Network, macAddress *vz.MACAddress) func(cfg *vz.VirtualMachineConfiguration) error {
	return func(cfg *vz.VirtualMachineConfiguration) error {
		var configurations []*vz.VirtioNetworkDeviceConfiguration
		attachment, err := vz.NewVmnetNetworkDeviceAttachment(network.Raw())
		if err != nil {
			return err
		}
		config, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
		if err != nil {
			return err
		}
		config.SetMACAddress(macAddress)
		configurations = append(configurations, config)
		cfg.SetNetworkDevicesVirtualMachineConfiguration(configurations)
		return nil
	}
}

// detectFreeIPv4Subnet detects a free IPv4 subnet on the host machine.
func detectFreeIPv4Subnet(t *testing.T, prefer netip.Prefix) netip.Prefix {
	targetPrefix := netip.MustParsePrefix("192.168.0.0/16")
	hostNetIfs, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	candidates := make([]netip.Prefix, len(hostNetIfs))
	for _, hostNetIf := range hostNetIfs {
		hostNetAddrs, err := hostNetIf.Addrs()
		if err != nil {
			continue
		}
		for _, hostNetAddr := range hostNetAddrs {
			netIPNet, ok := hostNetAddr.(*net.IPNet)
			if !ok {
				continue
			}
			hostPrefix := netip.MustParsePrefix(netIPNet.String())
			if targetPrefix.Overlaps(hostPrefix) {
				candidates = append(candidates, hostPrefix)
			}
		}
	}
	slices.SortFunc(candidates, func(l, r netip.Prefix) int {
		if l.Addr().Less(r.Addr()) {
			return -1
		}
		return 1
	})
	for _, candidate := range candidates {
		if prefer.Addr() != candidate.Addr() {
			return prefer
		}
	}
	t.Fatal("no free IPv4 subnet found")
	return netip.Prefix{}
}

// randomMACAddress generates a random locally administered MAC address.
func randomMACAddress(t *testing.T) *vz.MACAddress {
	mac, err := vz.NewRandomLocallyAdministeredMACAddress()
	if err != nil {
		t.Fatal(err)
	}
	return mac
}
