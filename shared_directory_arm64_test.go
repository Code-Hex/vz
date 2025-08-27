//go:build darwin && arm64
// +build darwin,arm64

package vz_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v3"
)

func TestLinuxRosettaAvailabilityString(t *testing.T) {
	cases := []struct {
		availability vz.LinuxRosettaAvailability
		want         string
	}{
		{
			availability: vz.LinuxRosettaAvailabilityNotSupported,
			want:         "LinuxRosettaAvailabilityNotSupported",
		},
		{
			availability: vz.LinuxRosettaAvailabilityNotInstalled,
			want:         "LinuxRosettaAvailabilityNotInstalled",
		},
		{
			availability: vz.LinuxRosettaAvailabilityInstalled,
			want:         "LinuxRosettaAvailabilityInstalled",
		},
	}
	for _, tc := range cases {
		got := tc.availability.String()
		if tc.want != got {
			t.Fatalf("want %q but got %q", tc.want, got)
		}
	}
}

func TestNewLinuxRosettaUnixSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaUnixSocketCachingOptions is supported from macOS 14")
	}
	dir := t.TempDir()
	t.Run("invalid filename length", func(t *testing.T) {
		filename := filepath.Join(dir, strings.Repeat("a", 150)) + ".txt"
		f, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		_, err = vz.NewLinuxRosettaUnixSocketCachingOptions(filename)
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); !strings.Contains(got, "maximum allowed length of") {
			t.Fatalf("unexpected error: %q", got)
		}
	})
}

func TestNewLinuxRosettaAbstractSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaAbstractSocketCachingOptions is supported from macOS 14")
	}
	t.Run("invalid name length", func(t *testing.T) {
		name := strings.Repeat("a", 350)
		_, err := vz.NewLinuxRosettaAbstractSocketCachingOptions(name)
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); !strings.Contains(got, "maximum allowed length of") {
			t.Fatalf("unexpected error: %q", got)
		}
	})
}

const (
	rosettaMountTag = "rosetta"
	helloMountTag   = "hello"
)

func prepareLinuxAmd64Hello(dir string) error {
	os.MkdirAll(dir, 0755)
	cmd := exec.Command("go", "mod", "init", "test/hello")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}
	contents := []byte(`
package main

import (
	"fmt"
	"runtime"
)

func main() {
    fmt.Println("Hello,", runtime.GOOS+"/"+runtime.GOARCH+"!")
}
`)
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), contents, 0644); err != nil {
		return err
	}
	cmd = exec.Command("go", "build", "-o", filepath.Join(dir, "hello"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	return cmd.Run()
}

func rosettaConfiguration(t *testing.T, o vz.LinuxRosettaCachingOptions) func(*vz.VirtualMachineConfiguration) error {
	// Setup Rosetta directory share
	rosettaDirectoryShare, err := vz.NewLinuxRosettaDirectoryShare()
	if err != nil {
		t.Fatal(err)
	}
	if o != nil {
		rosettaDirectoryShare.SetOptions(o)
	}
	rosettaConfig, err := vz.NewVirtioFileSystemDeviceConfiguration(rosettaMountTag)
	if err != nil {
		t.Fatal(err)
	}
	rosettaConfig.SetDirectoryShare(rosettaDirectoryShare)

	// Setup amd64 hello binary directory share
	helloPath := t.TempDir()
	if err := prepareLinuxAmd64Hello(helloPath); err != nil {
		t.Fatal(err)
	}
	helloSharedDirectory, err := vz.NewSharedDirectory(helloPath, true)
	if err != nil {
		t.Fatal(err)
	}
	helloDirectoryShare, err := vz.NewSingleDirectoryShare(helloSharedDirectory)
	if err != nil {
		t.Fatal(err)
	}
	helloConfig, err := vz.NewVirtioFileSystemDeviceConfiguration(helloMountTag)
	if err != nil {
		t.Fatal(err)
	}
	helloConfig.SetDirectoryShare(helloDirectoryShare)

	return func(vmc *vz.VirtualMachineConfiguration) error {
		vmc.SetDirectorySharingDevicesVirtualMachineConfiguration(
			[]vz.DirectorySharingDeviceConfiguration{
				rosettaConfig,
				helloConfig,
			},
		)
		return nil
	}
}

func (c *Container) exec(t *testing.T, cmds ...string) {
	t.Helper()
	for _, cmd := range cmds {
		session := c.NewSession(t)
		defer session.Close()
		output, err := session.CombinedOutput(cmd)
		if err != nil {
			if len(output) > 0 {
				t.Fatalf("failed to run command %q: %v, outputs:\n%s", cmd, err, string(output))
			} else {
				t.Fatalf("failed to run command %q: %v", cmd, err)
			}
		}
		if len(output) > 0 {
			t.Logf("command %q outputs:\n%s", cmd, string(output))
		}
	}
}

// rosettad's default unix socket
const rosettadDefaultUnixSocket = "~/.cache/rosettad/uds/rosetta.sock"

// Test Rosetta
// see: https://gist.github.com/arianvp/23bfd2a360116ac80c39f553cae56b3a

func TestRosettaWithoutCachingOptions(t *testing.T) {
	container := newVirtualizationMachine(t, rosettaConfiguration(t, nil))
	defer container.Shutdown()

	// Mount rosetta and hello directories
	container.exec(t, "mkdir -p /mnt/rosetta && mount -t virtiofs "+rosettaMountTag+" /mnt/rosetta")
	container.exec(t, "mkdir -p /mnt/hello && mount -t virtiofs "+helloMountTag+" /mnt/hello")

	// Execute hello binary using rosetta
	container.exec(t,
		"time /mnt/rosetta/rosetta /mnt/hello/hello",
		"echo No AOT caching && time /mnt/rosetta/rosetta /mnt/hello/hello",
	)
}

func TestRosettaWithAbstractSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaAbstractSocketCachingOptions is supported from macOS 14")
	}

	o, err := vz.NewLinuxRosettaAbstractSocketCachingOptions("rosetta-abs")
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, rosettaConfiguration(t, o))
	defer container.Shutdown()

	// Mount rosetta and hello directories
	container.exec(t, "mkdir -p /mnt/rosetta && mount -t virtiofs "+rosettaMountTag+" /mnt/rosetta")
	container.exec(t, "mkdir -p /mnt/hello && mount -t virtiofs "+helloMountTag+" /mnt/hello")

	// Launch rosettad daemon, then give it some time to create the socket if needed
	container.exec(t, "/mnt/rosetta/rosettad daemon&", "sleep 1")

	// Execute hello binary using rosetta
	container.exec(t,
		"time /mnt/rosetta/rosetta /mnt/hello/hello",
		"echo Expecting AOT cache hit on second run && time /mnt/rosetta/rosetta /mnt/hello/hello",
	)
}
func TestRosettaWithUnixSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaUnixSocketCachingOptions is supported from macOS 14")
	}

	rosettaUnixSocket := "/run/rosettad/rosetta.sock"
	o, err := vz.NewLinuxRosettaUnixSocketCachingOptions(rosettaUnixSocket)
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, rosettaConfiguration(t, o))
	defer container.Shutdown()

	// Mount rosetta and hello directories
	container.exec(t, "mkdir -p /mnt/rosetta && mount -t virtiofs "+rosettaMountTag+" /mnt/rosetta")
	container.exec(t, "mkdir -p /mnt/hello && mount -t virtiofs "+helloMountTag+" /mnt/hello")

	// Create a symlink configured rosetta socket pointing to the socket created by rosettad
	container.exec(t, "mkdir -p $(dirname "+rosettaUnixSocket+")", "ln -sf "+rosettadDefaultUnixSocket+" "+rosettaUnixSocket)

	// Launch rosettad daemon, then give it some time to create the socket if needed
	container.exec(t, "/mnt/rosetta/rosettad daemon&", "sleep 1")

	// Execute hello binary using rosetta
	container.exec(t,
		"time /mnt/rosetta/rosetta /mnt/hello/hello",
		"echo Expecting AOT cache hit on second run && time /mnt/rosetta/rosetta /mnt/hello/hello",
	)
}

// Test Rosetta behaviors
//
// - `TestRosettaBehaviorsWithoutCachingOptions`:
//     - Launching rosettad does not affect execution performance.
//
// - `TestRosettaBehaviorsWithAbstractSocketCachingOptions`:
//     - Without launching rosettad, there is no performance advantage.
//     - Launching rosettad makes the first execution slower, followed by faster executions.
//     - rosettad creates *.aotcache in the cache directory.
//
// - `TestRosettaBehaviorsWithUnixSocketCachingOptions`:
//     - Until creating a configured socket as a symlink to uds/rosetta.sock, caching does not work.
//     - The first execution is slower than without caching, but subsequent executions are faster.
//     - rosettad creates *.aotcache in the cache directory.
//     - rosetta creates *.flu files in the cache directory.
//
// see: [Rosetta AOT Caching on Linux for Virtualization.Framework](https://gist.github.com/arianvp/23bfd2a360116ac80c39f553cae56b3a)

func TestRosettaBehaviorsWithoutCachingOptions(t *testing.T) {
	container := newVirtualizationMachine(t, rosettaConfiguration(t, nil))
	defer container.Shutdown()

	// Mount rosetta and hello directories
	container.exec(t, "mkdir -p /mnt/rosetta && mount -t virtiofs "+rosettaMountTag+" /mnt/rosetta")
	container.exec(t, "mkdir -p /mnt/hello && mount -t virtiofs "+helloMountTag+" /mnt/hello")

	// Execute hello binary using rosetta
	container.exec(t, "time /mnt/rosetta/rosetta /mnt/hello/hello")

	// Launch rosettad daemon, then give it some time to create the socket if needed
	container.exec(t, "/mnt/rosetta/rosettad daemon&", "sleep 1")

	// Confirm that rosettad's default unix socket does not exist
	container.exec(t, "test ! -e "+rosettadDefaultUnixSocket)

	// Execute hello binary using rosetta again, expecting no caching
	container.exec(t, "echo No AOT caching && time /mnt/rosetta/rosetta /mnt/hello/hello")

	// Caching does not work even if rosettad is running
	container.exec(t, "test ! -f ~/.cache/rosetta/*")
	container.exec(t, "test ! -f ~/.cache/rosettad/*.aotcache")
}

func TestRosettaBehaviorsWithAbstractSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaAbstractSocketCachingOptions is supported from macOS 14")
	}

	o, err := vz.NewLinuxRosettaAbstractSocketCachingOptions("rosetta-abs")
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, rosettaConfiguration(t, o))
	defer container.Shutdown()

	// Mount rosetta and hello directories
	container.exec(t, "mkdir -p /mnt/rosetta && mount -t virtiofs "+rosettaMountTag+" /mnt/rosetta")
	container.exec(t, "mkdir -p /mnt/hello && mount -t virtiofs "+helloMountTag+" /mnt/hello")

	// Execute hello binary using rosetta
	container.exec(t, "time /mnt/rosetta/rosetta /mnt/hello/hello")

	// Launch rosettad daemon, then give it some time to create the socket if needed
	container.exec(t, "/mnt/rosetta/rosettad daemon&", "sleep 1")

	// Confirm that rosettad's default unix socket does not exist
	container.exec(t, "test ! -e "+rosettadDefaultUnixSocket)

	// Confirm that cache is empty
	container.exec(t, "test ! -f ~/.cache/rosetta/*.flu")
	container.exec(t, "test ! -f ~/.cache/rosettad/*.aotcache")

	// Execute hello binary using rosetta
	container.exec(t, "echo AOT caching makes execution slower on first run && time /mnt/rosetta/rosetta /mnt/hello/hello")

	// AOT caching works now
	container.exec(t, "test -f ~/.cache/rosettad/*.aotcache")

	// rosetta does not create .flu files when using abstract socket caching
	container.exec(t, "test ! -f ~/.cache/rosetta/*.flu")

	// Execute hello binary using rosetta again
	container.exec(t, "echo Expecting AOT cache hit on second run && time /mnt/rosetta/rosetta /mnt/hello/hello")
}

func TestRosettaBehaviorsWithUnixSocketCachingOptions(t *testing.T) {
	if vz.Available(14) {
		t.Skip("NewLinuxRosettaUnixSocketCachingOptions is supported from macOS 14")
	}

	rosettaUnixSocket := "/run/rosettad/rosetta.sock"
	o, err := vz.NewLinuxRosettaUnixSocketCachingOptions(rosettaUnixSocket)
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, rosettaConfiguration(t, o))
	defer container.Shutdown()

	// Mount rosetta and hello directories
	container.exec(t, "mkdir -p /mnt/rosetta && mount -t virtiofs "+rosettaMountTag+" /mnt/rosetta")
	container.exec(t, "mkdir -p /mnt/hello && mount -t virtiofs "+helloMountTag+" /mnt/hello")

	// Create the directory for the configured rosetta unix socket
	container.exec(t, "mkdir -p $(dirname "+rosettaUnixSocket+")")

	// Confirm that rosettad's default unix socket does not exist yet
	container.exec(t, "test ! -e "+rosettadDefaultUnixSocket)

	// Launch rosettad daemon, then give it some time to create the socket if needed
	container.exec(t, "/mnt/rosetta/rosettad daemon&", "sleep 1")

	// Confirm that rosettad's default unix socket is created by rosettad
	container.exec(t, "test -e "+rosettadDefaultUnixSocket)

	// Confirm that configured rosetta socket is not created
	container.exec(t, "test ! -e "+rosettaUnixSocket)

	// Execute hello binary using rosetta
	container.exec(t, "echo AOT caching makes execution slower on first run && time /mnt/rosetta/rosetta /mnt/hello/hello")

	// Caching does not work since configured rosetta socket does not exist
	container.exec(t, "test ! -f ~/.cache/rosetta/*.flu")
	container.exec(t, "test ! -f ~/.cache/rosettad/*.aotcache")

	// Create a symlink configured rosetta socket pointing to the socket created by rosettad
	container.exec(t, "ln -sf "+rosettadDefaultUnixSocket+" "+rosettaUnixSocket)

	// Execute hello binary using rosetta again
	container.exec(t, "time /mnt/rosetta/rosetta /mnt/hello/hello")

	// AOT caching works now
	container.exec(t, "test -f ~/.cache/rosettad/*.aotcache")

	// rosetta also creates .flu files when using unix socket caching
	container.exec(t, "test -f ~/.cache/rosetta/*.flu")

	// Execute hello binary using rosetta again
	container.exec(t, "echo Expecting AOT cache hit on second run && time /mnt/rosetta/rosetta /mnt/hello/hello")
}
