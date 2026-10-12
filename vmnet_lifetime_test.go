package vz

import (
	"net"
	"net/netip"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestVmnetNetworkDeviceAttachmentLifetime(t *testing.T) {
	if macOSAvailable(26) != nil {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, mode := range []struct {
		name string
		mode VmnetNetworkMode
	}{{"Host", VmnetNetworkModeHost}, {"Shared", VmnetNetworkModeShared}} {
		t.Run(mode.name, func(t *testing.T) {
			config := func() *VirtualMachineConfiguration {
				networkConfig, err := NewVmnetNetworkConfiguration(mode.mode)
				if err != nil {
					t.Fatal(err)
				}
				network, err := NewVmnetNetwork(networkConfig)
				if err != nil {
					t.Fatal(err)
				}
				attachment, err := NewVmnetNetworkDeviceAttachment(network)
				if err != nil {
					t.Fatal(err)
				}
				device, err := NewVirtioNetworkDeviceConfiguration(attachment)
				if err != nil {
					t.Fatal(err)
				}
				store, err := NewEFIVariableStore(filepath.Join(t.TempDir(), "nvram"), WithCreatingEFIVariableStore())
				if err != nil {
					t.Fatal(err)
				}
				boot, err := NewEFIBootLoader(WithEFIVariableStore(store))
				if err != nil {
					t.Fatal(err)
				}
				config, err := NewVirtualMachineConfiguration(boot, 1, 256*1024*1024)
				if err != nil {
					t.Fatal(err)
				}
				array := nativeObjectArray([]*VirtioNetworkDeviceConfiguration{device})
				vzbridge.VZVirtualMachineConfiguration_SetNetworkDevices(config, array)
				objc.Release(array)
				objc.Release(networkConfig.pointer)
				objc.Release(network.pointer)
				objc.Release(attachment.pointer)
				objc.Release(device.pointer)
				vzbridge.Drain()
				return config
			}()

			if valid, err := config.Validate(); !valid || err != nil {
				t.Fatal(valid, err)
			}
			machine, err := NewVirtualMachine(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := machine.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if machine.CanStop() {
					if err := machine.Stop(); err != nil {
						t.Error(err)
					}
				}
			})
			awaitNativeBridge(t, func() bool { return machine.State() == VirtualMachineStateRunning })
			if err := machine.Stop(); err != nil {
				t.Fatal(err)
			}
			awaitNativeBridge(t, func() bool { return machine.State() == VirtualMachineStateStopped })
		})
	}
}

func TestVmnetCopiedConfigurationAndNetwork(t *testing.T) {
	if macOSAvailable(26) != nil {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, collectedConfig := func() (VmnetNetworkConfiguration, <-chan struct{}) {
		original, err := NewVmnetNetworkConfiguration(VmnetNetworkModeHost)
		if err != nil {
			t.Fatal(err)
		}
		if err := original.SetIPv4Subnet(netip.MustParsePrefix("192.168.251.0/24")); err != nil {
			t.Fatal(err)
		}
		collected := make(chan struct{})
		runtime.AddCleanup(original, func(ch chan struct{}) { close(ch) }, collected)
		return *original, collected
	}()
	awaitNativeBridge(t, func() bool {
		select {
		case <-collectedConfig:
			return true
		default:
			return false
		}
	})
	network, collectedNetwork := func() (VmnetNetwork, <-chan struct{}) {
		original, err := NewVmnetNetwork(&config)
		if err != nil {
			t.Fatal(err)
		}
		collected := make(chan struct{})
		runtime.AddCleanup(original, func(ch chan struct{}) { close(ch) }, collected)
		return *original, collected
	}()
	awaitNativeBridge(t, func() bool {
		select {
		case <-collectedNetwork:
			return true
		default:
			return false
		}
	})
	if got, err := network.IPv4Subnet(); err != nil || got != netip.MustParsePrefix("192.168.251.0/24") {
		t.Fatalf("copied network subnet = %v, %v", got, err)
	}
	if err := config.AddDHCPReservation(net.HardwareAddr{2, 0, 0, 0, 0, 2}, netip.MustParseAddr("192.168.251.2")); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(config)
	runtime.KeepAlive(network)
}

func TestVmnetAttachmentNetworkRetainsReference(t *testing.T) {
	if macOSAvailable(26) != nil {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := NewVmnetNetworkConfiguration(VmnetNetworkModeHost)
	if err != nil {
		t.Fatal(err)
	}
	network, err := NewVmnetNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	want, err := network.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := NewVmnetNetworkDeviceAttachment(network)
	if err != nil {
		t.Fatal(err)
	}
	retained := attachment.Network()
	if retained == nil {
		t.Fatal("attachment returned no network")
	}
	objc.Release(config.pointer)
	objc.Release(network.pointer)
	objc.Release(attachment.pointer)
	vzbridge.Drain()
	if got, err := retained.IPv4Subnet(); err != nil || got != want {
		t.Fatalf("retained network subnet = %v, %v, want %v", got, err, want)
	}
}
