package vz_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v3"
)

func TestVirtioFileSystemDeviceConfigurationTag(t *testing.T) {
	if vz.Available(12) {
		t.Skip("VirtioFileSystemDeviceConfiguration is supported from macOS 12")
	}

	// The tag can’t be empty and must be fewer than 36 bytes when encoded in UTF-8.
	// https://developer.apple.com/documentation/virtualization/vzvirtiofilesystemdeviceconfiguration/3816092-validatetag?language=objc
	invalidTags := []string{
		"",
		strings.Repeat("a", 37),
	}
	for _, invalidTag := range invalidTags {
		_, err := vz.NewVirtioFileSystemDeviceConfiguration(invalidTag)
		if err == nil {
			t.Fatalf("want error for %q", invalidTag)
		}
	}
}

func TestSingleDirectoryShare(t *testing.T) {
	if vz.Available(12) {
		t.Skip("SingleDirectoryShare is supported from macOS 12")
	}

	cases := []struct {
		name     string
		readOnly bool
	}{
		{
			name:     "readonly",
			readOnly: true,
		},
		{
			name:     "read-write",
			readOnly: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sharedDirectory, err := vz.NewSharedDirectory(dir, tc.readOnly)
			if err != nil {
				t.Fatal(err)
			}
			single, err := vz.NewSingleDirectoryShare(sharedDirectory)
			if err != nil {
				t.Fatal(err)
			}

			tag := tc.name
			fileSystemDeviceConfig, err := vz.NewVirtioFileSystemDeviceConfiguration(tag)
			if err != nil {
				t.Fatal(err)
			}
			fileSystemDeviceConfig.SetDirectoryShare(single)

			container := newVirtualizationMachine(t,
				func(vmc *vz.VirtualMachineConfiguration) error {
					vmc.SetDirectorySharingDevicesVirtualMachineConfiguration(
						[]vz.DirectorySharingDeviceConfiguration{
							fileSystemDeviceConfig,
						},
					)
					return nil
				},
			)
			t.Cleanup(func() {
				if err := container.Shutdown(); err != nil {
					log.Println(err)
				}
			})

			file := "hello.txt"
			for _, v := range []struct {
				cmd     string
				wantErr bool
			}{
				{
					cmd:     "mkdir -p /mnt/shared",
					wantErr: false,
				},
				{
					cmd:     fmt.Sprintf("mount -t virtiofs %s /mnt/shared", tag),
					wantErr: false,
				},
				{
					cmd:     fmt.Sprintf("touch /mnt/shared/%s", file),
					wantErr: tc.readOnly,
				},
			} {
				session := container.NewSession(t)
				var buf bytes.Buffer
				session.Stderr = &buf
				if err := session.Run(v.cmd); err != nil && !v.wantErr {
					t.Fatalf("failed to run command %q: %v\nstderr: %q", v.cmd, err, buf)
				}
				session.Close()
			}

			if !tc.readOnly {
				_, err = os.Stat(filepath.Join(dir, file))
				if err != nil {
					t.Fatalf("expected the file to exist: %v", err)
				}
			}

			tmpFile := "tmp.txt"
			f, err := os.Create(filepath.Join(dir, tmpFile))
			if err != nil {
				t.Fatal(err)
			}
			f.Close()

			session := container.NewSession(t)
			defer session.Close()

			var buf bytes.Buffer
			session.Stderr = &buf
			check := "ls /mnt/shared/" + tmpFile
			if err := session.Run(check); err != nil {
				t.Fatalf("failed to run command %q: %v\nstderr: %q", check, err, buf)
			}
			session.Close()
		})
	}
}

func TestDirectorySharingDevices(t *testing.T) {
	if vz.Available(12) {
		t.Skip("VirtioFileSystemDevice is supported from macOS 12")
	}

	bootLoader, err := vz.NewLinuxBootLoader(
		"./testdata/Image",
		vz.WithCommandLine("console=hvc0"),
	)
	if err != nil {
		t.Fatalf("failed to create boot loader: %v", err)
	}

	config, err := vz.NewVirtualMachineConfiguration(bootLoader, 1, 256*1024*1024)
	if err != nil {
		t.Fatalf("failed to create virtual machine configuration: %v", err)
	}

	sharedDirectory, err := vz.NewSharedDirectory(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	single, err := vz.NewSingleDirectoryShare(sharedDirectory)
	if err != nil {
		t.Fatal(err)
	}
	fsConfig, err := vz.NewVirtioFileSystemDeviceConfiguration("share")
	if err != nil {
		t.Fatal(err)
	}
	fsConfig.SetDirectoryShare(single)
	config.SetDirectorySharingDevicesVirtualMachineConfiguration(
		[]vz.DirectorySharingDeviceConfiguration{fsConfig},
	)

	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		t.Fatalf("failed to create virtual machine: %v", err)
	}

	devices := vm.DirectorySharingDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 directory sharing device, got %d", len(devices))
	}
	if devices[0] == nil {
		t.Fatal("directory sharing device should not be nil")
	}
}

func TestVirtioFileSystemDeviceSetShare(t *testing.T) {
	if vz.Available(12) {
		t.Skip("VirtioFileSystemDevice is supported from macOS 12")
	}

	const tag = "shared"

	// Initial share: a single directory exposing fileA.
	dirA := t.TempDir()
	fileA := "a.txt"
	if f, err := os.Create(filepath.Join(dirA, fileA)); err != nil {
		t.Fatal(err)
	} else {
		f.Close()
	}
	sharedA, err := vz.NewSharedDirectory(dirA, false)
	if err != nil {
		t.Fatal(err)
	}
	single, err := vz.NewSingleDirectoryShare(sharedA)
	if err != nil {
		t.Fatal(err)
	}
	fsConfig, err := vz.NewVirtioFileSystemDeviceConfiguration(tag)
	if err != nil {
		t.Fatal(err)
	}
	fsConfig.SetDirectoryShare(single)

	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			vmc.SetDirectorySharingDevicesVirtualMachineConfiguration(
				[]vz.DirectorySharingDeviceConfiguration{fsConfig},
			)
			return nil
		},
	)
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	run := func(cmd string, wantErr bool) {
		t.Helper()
		session := container.NewSession(t)
		defer session.Close()
		var buf bytes.Buffer
		session.Stderr = &buf
		err := session.Run(cmd)
		switch {
		case err != nil && !wantErr:
			t.Fatalf("failed to run command %q: %v\nstderr: %q", cmd, err, buf)
		case err == nil && wantErr:
			t.Fatalf("expected command %q to fail but it succeeded", cmd)
		}
	}

	devices := container.DirectorySharingDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 directory sharing device, got %d", len(devices))
	}
	device := devices[0]

	// The running device reports the single share it was configured with.
	if got := device.Share(); got == nil {
		t.Fatal("expected a share on the running device, got nil")
	} else if _, ok := got.(*vz.SingleDirectoryShare); !ok {
		t.Fatalf("expected *vz.SingleDirectoryShare, got %T", got)
	}

	// The guest sees fileA through the initial share.
	run("mkdir -p /mnt/shared", false)
	run(fmt.Sprintf("mount -t virtiofs %s /mnt/shared", tag), false)
	run("ls /mnt/shared/"+fileA, false)

	// Swap to a multiple share exposing a different directory under "sub".
	dirB := t.TempDir()
	fileB := "b.txt"
	if f, err := os.Create(filepath.Join(dirB, fileB)); err != nil {
		t.Fatal(err)
	} else {
		f.Close()
	}
	sharedB, err := vz.NewSharedDirectory(dirB, false)
	if err != nil {
		t.Fatal(err)
	}
	multiple, err := vz.NewMultipleDirectoryShare(map[string]*vz.SharedDirectory{
		"sub": sharedB,
	})
	if err != nil {
		t.Fatal(err)
	}
	device.SetShare(multiple)

	// The running device now reports the swapped-in multiple share.
	if got := device.Share(); got == nil {
		t.Fatal("expected a share after swap, got nil")
	} else if _, ok := got.(*vz.MultipleDirectoryShare); !ok {
		t.Fatalf("expected *vz.MultipleDirectoryShare after swap, got %T", got)
	}

	// After remounting, the guest sees the swapped-in content and no longer fileA.
	run("umount /mnt/shared", false)
	run(fmt.Sprintf("mount -t virtiofs %s /mnt/shared", tag), false)
	run("ls /mnt/shared/sub/"+fileB, false)
	run("ls /mnt/shared/"+fileA, true)
}

func TestMultipleDirectoryShare(t *testing.T) {
	if vz.Available(12) {
		t.Skip("MultipleDirectoryShare is supported from macOS 12")
	}

	readOnlyDir := t.TempDir()
	readOnlySharedDirectory, err := vz.NewSharedDirectory(readOnlyDir, true)
	if err != nil {
		t.Fatal(err)
	}

	rwDir := t.TempDir()
	rwSharedDirectory, err := vz.NewSharedDirectory(rwDir, false)
	if err != nil {
		t.Fatal(err)
	}

	multiple, err := vz.NewMultipleDirectoryShare(map[string]*vz.SharedDirectory{
		"readonly":   readOnlySharedDirectory,
		"read_write": rwSharedDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}

	tag := "multiple"
	fileSystemDeviceConfig, err := vz.NewVirtioFileSystemDeviceConfiguration(tag)
	if err != nil {
		t.Fatal(err)
	}
	fileSystemDeviceConfig.SetDirectoryShare(multiple)

	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			vmc.SetDirectorySharingDevicesVirtualMachineConfiguration(
				[]vz.DirectorySharingDeviceConfiguration{
					fileSystemDeviceConfig,
				},
			)
			return nil
		},
	)
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	// Create a file in mount directories.
	tmpFile := "tmp.txt"
	for _, dir := range []string{readOnlyDir, rwDir} {
		f, err := os.Create(filepath.Join(dir, tmpFile))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	helloTxt := "hello.txt"
	for _, v := range []struct {
		cmd     string
		wantErr bool
	}{
		{
			cmd:     "mkdir -p /mnt/shared",
			wantErr: false,
		},
		{
			cmd:     fmt.Sprintf("mount -t virtiofs %s /mnt/shared", tag),
			wantErr: false,
		},
		{
			cmd:     fmt.Sprintf("ls /mnt/shared/readonly/%s", tmpFile),
			wantErr: false,
		},
		{
			cmd:     fmt.Sprintf("ls /mnt/shared/read_write/%s", tmpFile),
			wantErr: false,
		},
		{
			cmd:     fmt.Sprintf("touch /mnt/shared/readonly/%s", helloTxt),
			wantErr: true,
		},
		{
			cmd:     fmt.Sprintf("touch /mnt/shared/read_write/%s", helloTxt),
			wantErr: false,
		},
	} {
		session := container.NewSession(t)
		var buf bytes.Buffer
		session.Stderr = &buf
		if err := session.Run(v.cmd); err != nil && !v.wantErr {
			t.Fatalf("failed to run command %q: %v\nstderr: %q", v.cmd, err, buf)
		}
		session.Close()
	}

	_, err = os.Stat(filepath.Join(rwDir, helloTxt))
	if err != nil {
		t.Fatalf("expected the file to exist in read/write directory: %v", err)
	}
}
