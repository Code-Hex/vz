//go:build darwin && arm64

package vz

import "testing"

func TestRuntimeDirectorySharingRejectsRosetta(t *testing.T) {
	if macOSAvailable(12) != nil {
		t.Skip("requires macOS 12")
	}
	if err := (*VirtioFileSystemDevice)(nil).SetDirectoryShare(&LinuxRosettaDirectoryShare{}); err == nil {
		t.Fatal("accepted a Rosetta share at runtime")
	}
}

func TestRuntimeDirectorySharingReadsRosetta(t *testing.T) {
	if LinuxRosettaDirectoryShareAvailability() != LinuxRosettaAvailabilityInstalled {
		t.Skip("requires Rosetta for Linux")
	}
	share, err := NewLinuxRosettaDirectoryShare()
	if err != nil {
		t.Fatal(err)
	}
	fileSystem, err := NewVirtioFileSystemDeviceConfiguration("rosetta")
	if err != nil {
		t.Fatal(err)
	}
	fileSystem.SetDirectoryShare(share)
	config := newTestConfig(t)
	config.SetDirectorySharingDevicesVirtualMachineConfiguration([]DirectorySharingDeviceConfiguration{fileSystem})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	device := vm.DirectorySharingDevices()[0].(*VirtioFileSystemDevice)
	if got := device.DirectoryShare(); got == nil {
		t.Fatal("missing configured Rosetta share")
	} else if _, ok := got.(*LinuxRosettaDirectoryShare); !ok {
		t.Fatalf("directory share type = %T, want *LinuxRosettaDirectoryShare", got)
	}
}
