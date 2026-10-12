package vz

import (
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vmnethandle"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetNetworkDeviceAttachmentLifetime(t *testing.T) {
	if macOSAvailable(26) != nil {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, mode := range []struct {
		name string
		mode vmnet.Mode
	}{{"Host", vmnet.ModeHost}, {"Shared", vmnet.ModeShared}} {
		t.Run(mode.name, func(t *testing.T) {
			config := func() *VirtualMachineConfiguration {
				networkConfig, err := vmnet.NewNetworkConfiguration(mode.mode)
				if err != nil {
					t.Fatal(err)
				}
				network, err := vmnet.NewNetwork(networkConfig)
				if err != nil {
					t.Fatal(err)
				}
				attachment, err := NewVMNetNetworkDeviceAttachment(network)
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
				objc.Release(vmnethandle.Pointer(networkConfig))
				objc.Release(vmnethandle.Pointer(network))
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

func TestVMNetAttachmentNetworkRetainsReference(t *testing.T) {
	if macOSAvailable(26) != nil {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	want, err := network.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := NewVMNetNetworkDeviceAttachment(network)
	if err != nil {
		t.Fatal(err)
	}
	retained := attachment.Network()
	if retained == nil {
		t.Fatal("attachment returned no network")
	}
	objc.Release(vmnethandle.Pointer(config))
	objc.Release(vmnethandle.Pointer(network))
	objc.Release(attachment.pointer)
	vzbridge.Drain()
	if got, err := retained.IPv4Subnet(); err != nil || got != want {
		t.Fatalf("retained network subnet = %v, %v, want %v", got, err, want)
	}
}
