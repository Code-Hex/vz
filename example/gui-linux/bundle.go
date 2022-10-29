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
	return filepath.Join(home, "/GUI Linux VM.bundle/")
}

// GetMainDiskImagePath gets a path for disk image.
func GetMainDiskImagePath() string {
	return filepath.Join(GetVMBundlePath(), "Disk.img")
}

// GetEFIVariableStorePath gets a path for EFI variable store.
func GetEFIVariableStorePath() string {
	return filepath.Join(GetVMBundlePath(), "NVRAM")
}

// GetMachineIdentifierPath gets a path for machine identifier.
func GetMachineIdentifierPath() string {
	return filepath.Join(GetVMBundlePath(), "MachineIdentifier")
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
