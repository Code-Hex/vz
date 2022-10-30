package vz

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func newTestConfig(t *testing.T) *VirtualMachineConfiguration {
	f, err := os.CreateTemp("", "vmlinuz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	bootloader, err := NewLinuxBootLoader(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	config, err := NewVirtualMachineConfiguration(bootloader, 1, 1024*1024*1024)
	if err != nil {
		t.Fatal(err)
	}

	return config
}

func TestIssue50(t *testing.T) {
	config := newTestConfig(t)

	ok, err := config.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("failed to validate config")
	}
	m, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check for segmentation faults", func(t *testing.T) {
		cases := map[string]func() error{
			"start handler":  func() error { return m.Start() },
			"pause handler":  m.Pause,
			"resume handler": m.Resume,
			"stop handler":   m.Stop,
		}
		for name, run := range cases {
			t.Run(name, func(t *testing.T) {
				_ = run()
			})
		}
	})
}

func TestIssue43(t *testing.T) {
	const doesNotExists = "/non/existing/path"
	t.Run("does not throw NSInvalidArgumentException", func(t *testing.T) {
		t.Run("NewLinuxBootLoader", func(t *testing.T) {
			_, err := NewLinuxBootLoader(doesNotExists)
			if err == nil {
				t.Fatal("expected returns error")
			}
			if !strings.HasPrefix(err.Error(), "invalid linux kernel") {
				t.Error(err)
			}
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("want underlying error %q but got %q", os.ErrNotExist, err)
			}

			f, err := os.CreateTemp("", "vmlinuz")
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			_, err = NewLinuxBootLoader(f.Name(), WithInitrd(doesNotExists))
			if err == nil {
				t.Fatal("expected returns error")
			}
			if !strings.HasPrefix(err.Error(), "invalid initial RAM disk") {
				t.Error(err)
			}
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("want underlying error %q but got %q", os.ErrNotExist, err)
			}
		})

		cases := map[string]func() error{
			"NewSharedDirectory": func() error {
				_, err := NewSharedDirectory(doesNotExists, false)
				return err
			},
			"NewDiskImageStorageDeviceAttachment": func() error {
				_, err := NewDiskImageStorageDeviceAttachment(doesNotExists, false)
				return err
			},
		}
		for name, run := range cases {
			t.Run(name, func(t *testing.T) {
				err := run()
				if err == nil {
					t.Fatal("expected returns error")
				}
				if !errors.Is(err, ErrUnsupportedOSVersion) && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("want underlying error %q but got %q", os.ErrNotExist, err)
				}
			})
		}
	})
}

func TestIssue81(t *testing.T) {
	config := newTestConfig(t)

	ok, err := config.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("failed to validate config")
	}

	t.Run("SocketDevices", func(t *testing.T) {
		vsockDevs := config.SocketDevices()
		if len(vsockDevs) != 0 {
			t.Errorf("unexpected number of virtio-vsock devices: got %d, expected 0", len(vsockDevs))
		}

		vsockDev, err := NewVirtioSocketDeviceConfiguration()
		if err != nil {
			t.Fatal(err)
		}

		config.SetSocketDevicesVirtualMachineConfiguration([]SocketDeviceConfiguration{vsockDev})

		vsockDevs = config.SocketDevices()
		if len(vsockDevs) != 1 {
			t.Errorf("unexpected number of virtio-vsock devices: got %d, expected 1", len(vsockDevs))
		}
	})

	t.Run("NetworkDevices", func(t *testing.T) {
		networkDevs := config.NetworkDevices()
		if len(networkDevs) != 0 {
			t.Errorf("unexpected number of virtio-net devices: got %d, expected 0", len(networkDevs))
		}

		nat, err := NewNATNetworkDeviceAttachment()
		if err != nil {
			t.Fatal(err)
		}
		networkDev, err := NewVirtioNetworkDeviceConfiguration(nat)
		if err != nil {
			t.Fatal(err)
		}

		config.SetNetworkDevicesVirtualMachineConfiguration([]*VirtioNetworkDeviceConfiguration{networkDev})

		networkDevs = config.NetworkDevices()
		if len(networkDevs) != 1 {
			t.Errorf("unexpected number of virtio-vsock devices: got %d, expected 1", len(networkDevs))
		}
	})
}
