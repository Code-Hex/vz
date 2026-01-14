package vz_test

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v3"
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
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
	})

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
	if vz.Available(12) {
		t.Skip("vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync is supported from macOS 12")
	}

	devPath := ""
	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			dir := t.TempDir()
			path := filepath.Join(dir, "disk.img")
			if err := vz.CreateDiskImage(path, 512); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("hdiutil", "attach", "-imagekey", "diskimage-class=CRawDiskImage", "-nomount", path)
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("failed to attach disk image: %v", err)
			}

			outputStr := string(output)
			lines := strings.Split(outputStr, "\n")
			if len(lines) == 0 || !strings.HasPrefix(lines[0], "/dev/") {
				log.Printf("[%s]\n", lines)
				t.Fatalf("unexpected output from `hdiutil attach`")
			}
			if len(lines) != 0 && strings.HasPrefix(lines[0], "/dev/") {
				devPath = strings.TrimSpace(lines[0])
			}

			var attachment *vz.DiskBlockDeviceStorageDeviceAttachment
			{
				dev, err := os.Open(devPath)
				if err != nil {
					t.Fatal(err)
				}

				attachment, err = vz.NewDiskBlockDeviceStorageDeviceAttachment(dev, false, vz.DiskSynchronizationModeNone)
				if err != nil {
					t.Fatal(err)
				}
				if err := dev.Close(); err != nil {
					log.Printf("failed to close %s: %s\n", devPath, err)
				}

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
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
		cmd := exec.Command("hdiutil", "detach", devPath)
		if err := cmd.Run(); err != nil {
			log.Printf("hdiutil detach %s failed: %s\n", devPath, err)
		}
	})

	vm := container.VirtualMachine

	if got := vm.State(); vz.VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateRunning, got)
	}
}
