package vz_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/testhelper"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestBlockDeviceIdentifier(t *testing.T) {
	if vz.Available(12.3) {
		t.Skip("VirtioBlockDeviceConfiguration.SetBlockDeviceIdentifier is supported from macOS 12.3")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.img")
	if err := vz.CreateDiskImage(path, 512); err != nil {
		t.Fatal(err)
	}

	attachment, err := vz.NewDiskImageStorageDeviceAttachment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	got1, err := config.BlockDeviceIdentifier()
	if err != nil {
		t.Fatal(err)
	}
	if got1 != "" {
		t.Fatalf("want empty by default: %q", got1)
	}

	invalidID := strings.Repeat("h", 25)
	if err := config.SetBlockDeviceIdentifier(invalidID); err == nil {
		t.Fatal("want error")
	} else {
		nserr, ok := err.(*vz.NSError)
		if !ok {
			t.Fatalf("unexpected error: %v", err)
		}
		if nserr.Domain != "VZErrorDomain" {
			t.Errorf("unexpected NSError domain: %v", nserr)
		}
		if nserr.Code != int(vz.ErrorInvalidVirtualMachineConfiguration) {
			t.Errorf("unexpected NSError code: %v", nserr)
		}
	}

	want := "hello"
	if err := config.SetBlockDeviceIdentifier(want); err != nil {
		t.Fatal(err)
	}
	got2, err := config.BlockDeviceIdentifier()
	if err != nil {
		t.Fatal(err)
	}
	if got2 != want {
		t.Fatalf("want %q but got %q", want, got2)
	}
}

func TestBlockDeviceWithCacheAndSyncMode(t *testing.T) {
	if vz.Available(12) {
		t.Skip("vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync is supported from macOS 12")
	}

	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			dir := t.TempDir()
			path := filepath.Join(dir, "disk.img")
			if err := vz.CreateDiskImage(path, 512); err != nil {
				t.Fatal(err)
			}

			attachment, err := vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync(path, false, vz.DiskImageCachingModeCached, vz.DiskImageSynchronizationModeFsync)
			if err != nil {
				t.Fatal(err)
			}
			config, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
			if err != nil {
				t.Fatal(err)
			}
			vmc.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{
				config,
			})
			return nil
		},
	)

	vm := container.VirtualMachine

	if got := vm.State(); vz.VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateRunning, got)
	}
}

func TestBlockDeviceStorageDeviceAttachmentError(t *testing.T) {
	if vz.Available(14) {
		t.Skip("vz.NewDiskBlockDeviceStorageDeviceAttachment is supported from macOS 14")
	}

	f, err := os.Create(filepath.Join(t.TempDir(), "empty"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = vz.NewDiskBlockDeviceStorageDeviceAttachment(f, false, vz.DiskSynchronizationModeNone)
	if err == nil {
		t.Fatal("did not get an error with invalid file descriptor")
	}
}

func TestBlockDeviceWithDeviceAttachment(t *testing.T) {
	if vz.Available(14) {
		t.Skip("vz.NewDiskBlockDeviceStorageDeviceAttachment is supported from macOS 14")
	}

	const diskOperationTimeout = 10 * time.Second
	path := filepath.Join(t.TempDir(), "disk.img")
	if err := vz.CreateDiskImage(path, 512); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), diskOperationTimeout)
	cmd := exec.CommandContext(ctx, "hdiutil", "attach", "-imagekey", "diskimage-class=CRawDiskImage", "-nomount", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, attachErr := cmd.Output()
	cancel()
	fields := strings.Fields(string(output))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/dev/") {
		t.Fatalf("hdiutil attach: %v, output: %s, stderr: %s", attachErr, output, stderr.String())
	}
	devPath := fields[0]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), diskOperationTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "hdiutil", "detach", devPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("hdiutil detach %s: %v, output: %s", devPath, err, output)
		}
	})
	if attachErr != nil {
		t.Fatalf("hdiutil attach: %v, output: %s, stderr: %s", attachErr, output, stderr.String())
	}
	t.Cleanup(vzbridge.Drain)

	dev, err := os.Open(devPath)
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := vz.NewDiskBlockDeviceStorageDeviceAttachment(dev, false, vz.DiskSynchronizationModeNone)
	closeErr := dev.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release(attachment) })
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	config, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release(config) })

	var container *Container
	container, err = setupVirtualizationMachine(t, testhelper.NewSshConfig("root", "passwd"),
		func(vmc *vz.VirtualMachineConfiguration) error {
			t.Cleanup(func() { objc.Release(vmc) })
			// Stopping the VM does not release the device's duplicated file descriptor.
			t.Cleanup(func() {
				if container == nil {
					return
				}
				state := container.State()
				if state == vz.VirtualMachineStateStopped || state == vz.VirtualMachineStateError {
					objc.Release(container.VirtualMachine)
				}
			})
			vmc.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{
				config,
			})
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	vm := container.VirtualMachine

	if got := vm.State(); vz.VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateRunning, got)
	}
}
