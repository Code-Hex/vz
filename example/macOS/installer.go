package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Code-Hex/vz/v2"
)

func installMacOS(ctx context.Context) error {
	if err := CreateVMBundle(); err != nil {
		return fmt.Errorf("failed to VM.bundle in home directory: %w", err)
	}

	restoreImagePath := GetRestoreImagePath()
	if _, err := os.Stat(restoreImagePath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := downloadRestoreImage(ctx, restoreImagePath); err != nil {
			return fmt.Errorf("failed to download restore image: %w", err)
		}
	}
	restoreImage, err := vz.LoadMacOSRestoreImageFromPath(restoreImagePath)
	if err != nil {
		return fmt.Errorf("failed to load restore image: %w", err)
	}
	configurationRequirements := restoreImage.MostFeaturefulSupportedConfiguration()
	config, err := setupVirtualMachineWithMacOSConfigurationRequirements(
		configurationRequirements,
	)
	if err != nil {
		return fmt.Errorf("failed to setup config: %w", err)
	}
	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		return err
	}

	installer, err := vz.NewMacOSInstaller(vm, restoreImagePath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("install has been cancelled")
				return
			case <-installer.Done():
				fmt.Println("install has been completed")
				return
			case <-ticker.C:
				fmt.Printf("install: %.3f%%\r", installer.FractionCompleted()*100)
			}
		}
	}()

	return installer.Install(ctx)
}

func setupVirtualMachineWithMacOSConfigurationRequirements(macOSConfiguration *vz.MacOSConfigurationRequirements) (*vz.VirtualMachineConfiguration, error) {
	platformConfig, err := createMacInstallerPlatformConfiguration(macOSConfiguration)
	if err != nil {
		return nil, fmt.Errorf("failed to create mac platform config: %w", err)
	}
	return setupVMConfiguration(platformConfig)
}

func createMacInstallerPlatformConfiguration(macOSConfiguration *vz.MacOSConfigurationRequirements) (*vz.MacPlatformConfiguration, error) {
	hardwareModel := macOSConfiguration.HardwareModel()
	if err := CreateFileAndWriteTo(
		hardwareModel.DataRepresentation(),
		GetHardwareModelPath(),
	); err != nil {
		return nil, fmt.Errorf("failed to write hardware model data: %w", err)
	}

	machineIdentifier, err := vz.NewMacMachineIdentifier()
	if err != nil {
		return nil, err
	}
	if err := CreateFileAndWriteTo(
		machineIdentifier.DataRepresentation(),
		GetMachineIdentifierPath(),
	); err != nil {
		return nil, fmt.Errorf("failed to write machine identifier data: %w", err)
	}

	auxiliaryStorage, err := vz.NewMacAuxiliaryStorage(
		GetAuxiliaryStoragePath(),
		vz.WithCreatingStorage(hardwareModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new mac auxiliary storage: %w", err)
	}
	return vz.NewMacPlatformConfiguration(
		vz.WithAuxiliaryStorage(auxiliaryStorage),
		vz.WithHardwareModel(hardwareModel),
		vz.WithMachineIdentifier(machineIdentifier),
	)
}

func downloadRestoreImage(ctx context.Context, destPath string) error {
	progress, err := vz.FetchLatestSupportedMacOSRestoreImage(ctx, destPath)
	if err != nil {
		return err
	}

	fmt.Printf("download restore image in %q\n", destPath)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("download has been cancelled")
			return ctx.Err()
		case <-progress.Finished():
			fmt.Println("download has been completed")
			return progress.Err()
		case <-ticker.C:
			fmt.Printf("download: %.3f%%\r", progress.FractionCompleted()*100)
		}
	}
}
