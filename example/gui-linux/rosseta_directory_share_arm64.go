//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"log"

	"github.com/Code-Hex/vz/v2"
	"github.com/Songmu/prompter"
)

func createRosettaDirectoryShareConfiguration() (*vz.VirtioFileSystemDeviceConfiguration, error) {
	config, err := vz.NewVirtioFileSystemDeviceConfiguration("vz-rosetta")
	if err != nil {
		return nil, fmt.Errorf("failed to create a new virtio file system configuration: %w", err)
	}
	availability := vz.LinuxRosettaDirectoryShareAvailability()
	switch availability {
	case vz.LinuxRosettaAvailabilityNotSupported:
		return nil, fmt.Errorf("not supported rosetta: %w", errIgnoreInstall)
	case vz.LinuxRosettaAvailabilityNotInstalled:
		want := prompter.YN("Do you want to install rosetta?", false)
		if !want {
			return nil, errIgnoreInstall
		}
		log.Println("installing rosetta...")
		if err := vz.LinuxRosettaDirectoryShareInstallRosetta(); err != nil {
			return nil, fmt.Errorf("failed to install rosetta: %w", err)
		}
		log.Println("complete.")
	case vz.LinuxRosettaAvailabilityInstalled:
		// nothing to do
	}

	rosettaShare, err := vz.NewLinuxRosettaDirectoryShare()
	if err != nil {
		return nil, fmt.Errorf("failed to create a new rosetta directory share: %w", err)
	}
	config.SetDirectoryShare(rosettaShare)

	return config, nil
}
