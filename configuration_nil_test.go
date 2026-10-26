package vz_test

import (
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestConfigurationRejectsNilDevices(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	boot, err := vz.NewEFIBootLoader()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		create func() error
	}{
		{"custom device", func() error {
			_, err := vz.NewVirtualMachineConfiguration(boot, 1, 1024*1024*1024, vz.WithCustomVirtioDevices(nil))
			return err
		}},
		{"zero custom device", func() error {
			_, err := vz.NewVirtualMachineConfiguration(boot, 1, 1024*1024*1024, vz.WithCustomVirtioDevices(&vz.CustomVirtioDeviceConfiguration{}))
			return err
		}},
		{"memory region", func() error {
			_, err := vz.NewCustomVirtioDeviceConfiguration(4, 0xff, 0, 1, vz.WithCustomVirtioSharedMemoryRegions(nil))
			return err
		}},
		{"zero memory region", func() error {
			_, err := vz.NewCustomVirtioDeviceConfiguration(4, 0xff, 0, 1, vz.WithCustomVirtioSharedMemoryRegions(&vz.VirtioSharedMemoryRegionConfiguration{}))
			return err
		}},
		{"USB mass storage", func() error {
			_, err := vz.NewXHCIControllerConfiguration(vz.WithUSBDevices((*vz.USBMassStorageDeviceConfiguration)(nil)))
			return err
		}},
		{"USB passthrough", func() error {
			_, err := vz.NewXHCIControllerConfiguration(vz.WithUSBDevices((*vz.USBPassthroughDeviceConfiguration)(nil)))
			return err
		}},
		{"zero USB mass storage", func() error {
			_, err := vz.NewXHCIControllerConfiguration(vz.WithUSBDevices(&vz.USBMassStorageDeviceConfiguration{}))
			return err
		}},
		{"zero USB passthrough", func() error {
			_, err := vz.NewXHCIControllerConfiguration(vz.WithUSBDevices(&vz.USBPassthroughDeviceConfiguration{}))
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.create(); err == nil {
				t.Fatal("invalid configuration succeeded")
			}
		})
	}
}
