package vz_test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v4"
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
					t.Fatalf("failed to run command %q: %v\nstderr: %q", v.cmd, err, buf.String())
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
				t.Fatalf("failed to run command %q: %v\nstderr: %q", check, err, buf.String())
			}
			session.Close()
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
			t.Fatalf("failed to run command %q: %v\nstderr: %q", v.cmd, err, buf.String())
		}
		session.Close()
	}

	_, err = os.Stat(filepath.Join(rwDir, helloTxt))
	if err != nil {
		t.Fatalf("expected the file to exist in read/write directory: %v", err)
	}
}

func TestMultipleDirectoryShareNames(t *testing.T) {
	if vz.Available(12) {
		t.Skip("MultipleDirectoryShare is supported from macOS 12")
	}
	shared, err := vz.NewSharedDirectory(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		nativeError bool
	}{
		{name: "", nativeError: true},
		{name: ".", nativeError: true},
		{name: "..", nativeError: true},
		{name: "a/b", nativeError: true},
		{name: strings.Repeat("a", 256), nativeError: true},
		{name: "a\x00b"},
		{name: "\xff"},
	} {
		t.Run(fmt.Sprintf("invalid %q", tc.name), func(t *testing.T) {
			name := tc.name
			share, err := vz.NewMultipleDirectoryShare(map[string]*vz.SharedDirectory{name: shared})
			if share != nil || err == nil {
				t.Fatalf("got (%v, %v), want (nil, error)", share, err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", name)) {
				t.Fatalf("error does not identify name %q: %v", name, err)
			}
			var nativeError *vz.NSError
			if tc.nativeError && !errors.As(err, &nativeError) {
				t.Fatalf("expected NSError, got %T: %v", err, err)
			}
		})
	}
	for _, shares := range []map[string]*vz.SharedDirectory{
		{},
		{"documents": shared, "共有": shared},
	} {
		share, err := vz.NewMultipleDirectoryShare(shares)
		if err != nil || share == nil {
			t.Fatalf("got (%v, %v), want a share without an error", share, err)
		}
	}
}

func TestDirectoryShareInvalidDirectory(t *testing.T) {
	if vz.Available(12) {
		t.Skip("Directory sharing is supported from macOS 12")
	}

	for _, tc := range []struct {
		name      string
		directory *vz.SharedDirectory
	}{
		{name: "nil"},
		{name: "zero", directory: &vz.SharedDirectory{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("single", func(t *testing.T) {
				share, err := vz.NewSingleDirectoryShare(tc.directory)
				if share != nil || err == nil {
					t.Fatalf("got (%v, %v), want (nil, error)", share, err)
				}
			})
			t.Run("multiple", func(t *testing.T) {
				share, err := vz.NewMultipleDirectoryShare(map[string]*vz.SharedDirectory{
					"documents": tc.directory,
				})
				if share != nil || err == nil {
					t.Fatalf("got (%v, %v), want (nil, error)", share, err)
				}
				if !strings.Contains(err.Error(), "documents") {
					t.Fatalf("error does not identify the directory: %v", err)
				}
			})
		})
	}
}

func TestCanonicalizeDirectoryShareName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "unchanged", input: "Shared Documents", want: "Shared Documents"},
		{name: "unsafe characters", input: `a*/:<>?\|b`, want: "ab"},
		{name: "preserved punctuation", input: " .hidden\t\n\" ", want: " .hidden\t\n\" "},
		{name: "unicode", input: "日本語😀e\u0301", want: "日本語😀e\u0301"},
		{name: "leading BOM", input: "\uFEFFname", want: "name"},
		{name: "BOM after unsafe character", input: "/\uFEFFname", want: "name"},
		{name: "two leading BOMs", input: "\uFEFF\uFEFFname", want: "\uFEFFname"},
		{name: "internal BOM", input: "a\uFEFFb", want: "a\uFEFFb"},
		{name: "BOM becomes empty", input: "\uFEFF", want: ""},
		{name: "BOM becomes dot", input: "/\uFEFF.", want: "."},
		{name: "BOM becomes dot dot", input: "\uFEFF..", want: ".."},
		{name: "maximum bytes with BOM", input: "\uFEFF" + strings.Repeat("a", 252), want: strings.Repeat("a", 252)},
		{name: "too long before BOM removal", input: "\uFEFF" + strings.Repeat("a", 253), wantErr: true},
		{name: "empty", wantErr: true},
		{name: "only unsafe characters", input: `*/:<>?\|`, wantErr: true},
		{name: "dot", input: "/.", wantErr: true},
		{name: "dot dot", input: ".:/.", wantErr: true},
		{name: "three dots", input: "...", want: "..."},
		{name: "maximum bytes", input: strings.Repeat("a", 255), want: strings.Repeat("a", 255)},
		{name: "too long", input: strings.Repeat("a", 256), wantErr: true},
		{name: "multibyte maximum", input: strings.Repeat("😀", 63) + "abc", want: strings.Repeat("😀", 63) + "abc"},
		{name: "multibyte too long", input: strings.Repeat("😀", 64), wantErr: true},
		{name: "length after removal", input: strings.Repeat("/", 256) + "name", want: "name"},
		{name: "invalid UTF-8", input: "a\xffb", wantErr: true},
		{name: "NUL", input: "a\x00b", want: "a"},
		{name: "leading NUL", input: "\x00a", wantErr: true},
		{name: "NUL after unsafe characters", input: "a/\x00b", want: "a"},
		{name: "NUL before unsafe characters", input: "a\x00/b", want: "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := vz.CanonicalizeDirectoryShareName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("name = %q, want %q", got, tc.want)
			}
		})
	}
}
