package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/Code-Hex/vz/v2"
)

func main() {
	// progressReader, err := vz.FetchLatestSupportedMacOSRestoreImage(context.Background(), "RestoreImage.ipsw")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// ticker := time.NewTicker(time.Millisecond * 500)
	// defer ticker.Stop()
	// for {
	// 	select {
	// 	case <-ticker.C:
	// 		log.Printf("progress: %f", progressReader.FractionCompleted()*100)
	// 	case <-progressReader.Finished():
	// 		log.Println("finished", progressReader.Err())
	// 		return
	// 	}
	// }

	restoreImage, err := vz.LoadMacOSRestoreImageFromPath(GetRestoreImagePath())

	log.Println(restoreImage.BuildVersion())
	log.Println(restoreImage.URL())
	log.Println(restoreImage.OperatingSystemVersion())
	config := restoreImage.MostFeaturefulSupportedConfiguration()
	hardwareModel := config.HardwareModel()
	log.Println(hardwareModel.Supported(), string(hardwareModel.DataRepresentation()))
	log.Println(err, "err == nil", err == nil)

}

func run(ctx context.Context) error {
	restoreImage, err := vz.LoadMacOSRestoreImageFromPath(GetRestoreImagePath())
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
	vm := vz.NewVirtualMachine(config)
	var _ = vm

	return nil
}

func setupVirtualMachineWithMacOSConfigurationRequirements(macOSConfiguration *vz.MacOSConfigurationRequirements) (*vz.VirtualMachineConfiguration, error) {
	config := vz.NewVirtualMachineConfiguration(
		vz.NewMacOSBootLoader(),
		computeCPUCount(),
		computeMemorySize(),
	)
	platformConfig, err := createMacPlatformConfiguration(macOSConfiguration)
	if err != nil {
		return nil, fmt.Errorf("failed to create mac platform config: %w", err)
	}
	config.SetPlatformVirtualMachineConfiguration(platformConfig)
	config.SetGraphicsDevicesVirtualMachineConfiguration([]vz.GraphicsDeviceConfiguration{
		createGraphicsDeviceConfiguration(),
	})
	blockDeviceConfig, err := createBlockDeviceConfiguration(GetDiskImagePath())
	if err != nil {
		return nil, fmt.Errorf("failed to create block device configuration: %w", err)
	}
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{blockDeviceConfig})

	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{
		createNetworkDeviceConfiguration(),
	})

	config.SetPointingDevicesVirtualMachineConfiguration([]vz.PointingDeviceConfiguration{
		createPointingDeviceConfiguration(),
	})

	config.SetKeyboardsVirtualMachineConfiguration([]vz.KeyboardConfiguration{
		createKeyboardConfiguration(),
	})

	config.SetAudioDevicesVirtualMachineConfiguration([]vz.AudioDeviceConfiguration{
		createAudioDeviceConfiguration(),
	})

	validated, err := config.Validate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate configuration: %w", err)
	}
	if !validated {
		return nil, fmt.Errorf("invalid configuration")
	}

	return config, nil
}

func computeCPUCount() uint {
	totalAvailableCPUs := runtime.NumCPU()
	virtualCPUCount := uint(totalAvailableCPUs - 1)
	if virtualCPUCount <= 1 {
		virtualCPUCount = 1
	}
	// TODO(codehex): use generics function when deprecated Go 1.17
	maxAllowed := vz.VirtualMachineConfigurationMaximumAllowedCPUCount()
	if virtualCPUCount > maxAllowed {
		virtualCPUCount = maxAllowed
	}
	minAllowed := vz.VirtualMachineConfigurationMinimumAllowedCPUCount()
	if virtualCPUCount < minAllowed {
		virtualCPUCount = minAllowed
	}
	return virtualCPUCount
}

func computeMemorySize() uint64 {
	// We arbitrarily choose 4GB.
	memorySize := uint64(4 * 1024 * 1024 * 1024)
	maxAllowed := vz.VirtualMachineConfigurationMaximumAllowedMemorySize()
	if memorySize > maxAllowed {
		memorySize = maxAllowed
	}
	minAllowed := vz.VirtualMachineConfigurationMinimumAllowedMemorySize()
	if memorySize < minAllowed {
		memorySize = minAllowed
	}
	return memorySize
}

func createBlockDeviceConfiguration(diskPath string) (*vz.VirtioBlockDeviceConfiguration, error) {
	// create disk image with 64 GiB
	if err := vz.CreateDiskImage(diskPath, 64*1024*1024*1024); err != nil {
		return nil, fmt.Errorf("failed to create disk image: %w", err)
	}
	diskImageAttachment, err := vz.NewDiskImageStorageDeviceAttachment(
		diskPath,
		false,
	)
	if err != nil {
		return nil, err
	}
	storageDeviceConfig := vz.NewVirtioBlockDeviceConfiguration(diskImageAttachment)
	return storageDeviceConfig, nil
}

func createMacPlatformConfiguration(macOSConfiguration *vz.MacOSConfigurationRequirements) (*vz.MacPlatformConfiguration, error) {
	hardwareModel := macOSConfiguration.HardwareModel()
	if err := CreateFileAndWriteTo(
		hardwareModel.DataRepresentation(),
		GetHardwareModelPath(),
	); err != nil {
		return nil, fmt.Errorf("failed to write hardware model data: %w", err)
	}

	machineIdentifier := vz.NewMacMachineIdentifier()
	if err := CreateFileAndWriteTo(
		machineIdentifier.DataRepresentation(),
		GetMachineIdentifierPath(),
	); err != nil {
		return nil, fmt.Errorf("failed to write machine identifier data: %w", err)
	}

	auxiliaryStorage, err := vz.NewMacAuxiliaryStorage(
		GetAuxiliaryStoragePath(),
		vz.WithCreating(hardwareModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new mac auxiliary storage: %w", err)
	}
	return vz.NewMacPlatformConfiguration(
		vz.WithAuxiliaryStorage(auxiliaryStorage),
		vz.WithHardwareModel(hardwareModel),
		vz.WithMachineIdentifier(machineIdentifier),
	), nil
}

func createGraphicsDeviceConfiguration() *vz.MacGraphicsDeviceConfiguration {
	graphicDeviceConfig := vz.NewMacGraphicsDeviceConfiguration()
	graphicDeviceConfig.SetDisplays(
		vz.NewMacGraphicsDisplayConfiguration(1920, 1200, 80),
	)
	return graphicDeviceConfig
}

func createNetworkDeviceConfiguration() *vz.VirtioNetworkDeviceConfiguration {
	natAttachment := vz.NewNATNetworkDeviceAttachment()
	networkConfig := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	return networkConfig
}

func createPointingDeviceConfiguration() *vz.USBScreenCoordinatePointingDeviceConfiguration {
	return vz.NewUSBScreenCoordinatePointingDeviceConfiguration()
}

func createKeyboardConfiguration() *vz.USBKeyboardConfiguration {
	return vz.NewUSBKeyboardConfiguration()
}

func createAudioDeviceConfiguration() *vz.VirtioSoundDeviceConfiguration {
	audioConfig := vz.NewVirtioSoundDeviceConfiguration()
	inputStream := vz.NewVirtioSoundDeviceHostInputStreamConfiguration()
	outputStream := vz.NewVirtioSoundDeviceHostOutputStreamConfiguration()
	audioConfig.SetStreams(
		inputStream,
		outputStream,
	)
	return audioConfig
}
