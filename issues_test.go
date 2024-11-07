package vz

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3/internal/objc"
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

func TestIssue96(t *testing.T) {
	t.Run("non-network fd", func(t *testing.T) {
		t.Parallel()
		f, err := os.CreateTemp("", "*")
		if err != nil {
			t.Fatal(err)
		}

		_, err = NewFileHandleNetworkDeviceAttachment(f)
		if err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("unix socket", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "issue96.sock")
		ln, err := net.ListenUnix("unix", &net.UnixAddr{
			Name: path,
			Net:  "unix",
		})
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		f, err := ln.File()
		if err != nil {
			t.Fatal(err)
		}

		_, err = NewFileHandleNetworkDeviceAttachment(f)
		if err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("TCP socket", func(t *testing.T) {
		t.Parallel()
		ln, err := net.ListenTCP("tcp", &net.TCPAddr{
			Port: 0,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		f, err := ln.File()
		if err != nil {
			t.Fatal(err)
		}

		_, err = NewFileHandleNetworkDeviceAttachment(f)
		if err == nil {
			t.Fatal("want error")
		}
	})
}

// unixgram must be supported
func TestIssue98(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "issue98.sock")
	ln, err := net.ListenUnixgram("unixgram", &net.UnixAddr{
		Name: path,
		Net:  "unix",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	f, err := ln.File()
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewFileHandleNetworkDeviceAttachment(f)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIssue119(t *testing.T) {
	vmlinuz := "./testdata/Image"
	initramfs := "./testdata/initramfs.cpio.gz"
	bootLoader, err := NewLinuxBootLoader(
		vmlinuz,
		WithCommandLine("console=hvc0"),
		WithInitrd(initramfs),
	)
	if err != nil {
		t.Fatal(err)
	}

	config, err := setupIssue119Config(bootLoader)
	if err != nil {
		t.Fatal(err)
	}

	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}

	if canStart := vm.CanStart(); !canStart {
		t.Fatal("want CanStart is true")
	}

	if err := vm.Start(); err != nil {
		t.Fatal(err)
	}

	if got := vm.State(); VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", VirtualMachineStateRunning, got)
	}

	// Simulates Go's VirtualMachine struct has been destructured but
	// Objective-C VZVirtualMachine object has not been destructured.
	objc.Retain(vm.pointer)
	vm.finalize()

	// sshSession.Run("poweroff")
	if vm.CanStop() {
		if err := vm.Stop(); err != nil {
			t.Error(err)
		}
	}
	if vm.CanRequestStop() {
		if _, err := vm.RequestStop(); err != nil {
			t.Error(err)
		}
	}
	timer := time.After(3 * time.Second)
	for {
		select {
		case state := <-vm.StateChangedNotify():
			if VirtualMachineStateStopped == state {
				return
			}
		case <-timer:
			t.Fatal("failed to shutdown vm")
		}
	}
}

func setupIssue119Config(bootLoader *LinuxBootLoader) (*VirtualMachineConfiguration, error) {
	config, err := NewVirtualMachineConfiguration(
		bootLoader,
		1,
		512*1024*1024,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new virtual machine config: %w", err)
	}

	// entropy device
	entropyConfig, err := NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create entropy device config: %w", err)
	}
	config.SetEntropyDevicesVirtualMachineConfiguration([]*VirtioEntropyDeviceConfiguration{
		entropyConfig,
	})

	// memory balloon device
	memoryBalloonDevice, err := NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create memory balloon device config: %w", err)
	}
	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]MemoryBalloonDeviceConfiguration{
		memoryBalloonDevice,
	})

	if _, err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}
