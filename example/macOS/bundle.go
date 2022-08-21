package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateVMBundle creates macOS VM bundle path if not exists.
func CreateVMBundle() error {
	return os.MkdirAll(GetVMBundlePath(), 0777)
}

// GetVMBundlePath gets macOS VM bundle path.
func GetVMBundlePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err) //
	}
	return filepath.Join(home, "/VM.bundle/")
}

// GetAuxiliaryStoragePath gets a path for auxiliary storage.
func GetAuxiliaryStoragePath() string {
	return filepath.Join(GetVMBundlePath(), "AuxiliaryStorage")
}

// GetDiskImagePath gets a path for disk image.
func GetDiskImagePath() string {
	return filepath.Join(GetVMBundlePath(), "Disk.img")
}

// GetHardwareModelPath gets a path for hardware model.
func GetHardwareModelPath() string {
	return filepath.Join(GetVMBundlePath(), "HardwareModel")
}

// GetMachineIdentifierPath gets a path for machine identifier.
func GetMachineIdentifierPath() string {
	return filepath.Join(GetVMBundlePath(), "MachineIdentifier")
}

// GetRestoreImagePath gets a path for restore image file.
func GetRestoreImagePath() string {
	return filepath.Join(GetVMBundlePath(), "RestoreImage.ipsw")
}

// CreateFileAndWriteTo creates a new file and write data to it.
func CreateFileAndWriteTo(data []byte, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", path, err)
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}
