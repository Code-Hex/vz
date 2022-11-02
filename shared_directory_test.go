package vz_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			defer container.Close()

			vm := container.VirtualMachine

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

			if err := vm.Stop(); err != nil {
				t.Fatal(err)
			}

			timeout := 3 * time.Second
			waitState(t, timeout, vm, vz.VirtualMachineStateStopping)
			waitState(t, timeout, vm, vz.VirtualMachineStateStopped)
		})
	}
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
	defer container.Close()

	vm := container.VirtualMachine

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

	if err := vm.Stop(); err != nil {
		t.Fatal(err)
	}

	timeout := 3 * time.Second
	waitState(t, timeout, vm, vz.VirtualMachineStateStopping)
	waitState(t, timeout, vm, vz.VirtualMachineStateStopped)
}
