package vz_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestRuntimeDirectorySharingGuest(t *testing.T) {
	if vz.Available(12) {
		t.Skip("requires macOS 12")
	}
	shares := make([]*vz.SingleDirectoryShare, 2)
	for i, marker := range []string{"first", "second"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "marker"), []byte(marker), 0644); err != nil {
			t.Fatal(err)
		}
		shared, err := vz.NewSharedDirectory(dir, true)
		if err != nil {
			t.Fatal(err)
		}
		shares[i], err = vz.NewSingleDirectoryShare(shared)
		if err != nil {
			t.Fatal(err)
		}
	}
	fs, err := vz.NewVirtioFileSystemDeviceConfiguration("runtime-share")
	if err != nil {
		t.Fatal(err)
	}
	fs.SetDirectoryShare(shares[0])
	vm := newVirtualizationMachine(t, func(c *vz.VirtualMachineConfiguration) error {
		c.SetDirectorySharingDevicesVirtualMachineConfiguration([]vz.DirectorySharingDeviceConfiguration{fs})
		return nil
	})
	t.Cleanup(func() {
		if err := vm.Shutdown(); err != nil {
			t.Error(err)
		}
	})
	run := func(command string) string {
		t.Helper()
		session := vm.NewSession(t)
		defer session.Close()
		output, err := session.CombinedOutput(command)
		if err != nil {
			t.Fatalf("%s: %v\n%s", command, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("mkdir -p /mnt/runtime-share; mount -t virtiofs runtime-share /mnt/runtime-share")
	if got := run("cat /mnt/runtime-share/marker"); got != "first" {
		t.Fatalf("initial marker = %q", got)
	}
	run("umount /mnt/runtime-share")
	devices := vm.DirectorySharingDevices()
	if len(devices) != 1 {
		t.Fatalf("got %d directory sharing devices, want 1", len(devices))
	}
	device := devices[0].(*vz.VirtioFileSystemDevice)
	if err := device.SetDirectoryShare(shares[1]); err != nil {
		t.Fatal(err)
	}
	run("mount -t virtiofs runtime-share /mnt/runtime-share")
	if got := run("cat /mnt/runtime-share/marker"); got != "second" {
		t.Fatalf("replacement marker = %q", got)
	}
	run("umount /mnt/runtime-share")
	if err := device.SetDirectoryShare(nil); err != nil {
		t.Fatal(err)
	}
	run("if mount -t virtiofs runtime-share /mnt/runtime-share; then test ! -e /mnt/runtime-share/marker || exit 1; umount /mnt/runtime-share; fi")
}
