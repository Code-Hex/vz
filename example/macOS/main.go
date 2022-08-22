package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

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

	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if false {
		defer time.Sleep(time.Second)
		return installMacOS(ctx)
	}
	return runVM(ctx)
}

func runVM(ctx context.Context) error {
	platformConfig, err := createMacPlatformConfiguration()
	if err != nil {
		return err
	}
	config, err := setupVMConfiguration(platformConfig)
	if err != nil {
		return err
	}
	vm := vz.NewVirtualMachine(config)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM)

	errCh := make(chan error, 1)

	vm.Start(func(err error) {
		if err != nil {
			errCh <- err
		}
	})

	for {
		select {
		case <-signalCh:
			result, err := vm.RequestStop()
			if err != nil {
				return err
			}
			log.Println("recieved signal", result)
		case newState := <-vm.StateChangedNotify():
			if newState == vz.VirtualMachineStateRunning {
				log.Println("start VM is running")
			}
			if newState == vz.VirtualMachineStateStopped {
				log.Println("stopped successfully")
				return nil
			}
		case err := <-errCh:
			return fmt.Errorf("failed to start vm: %w", err)
		}
	}
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
		if !os.IsExist(err) {
			return nil, fmt.Errorf("failed to create disk image: %w", err)
		}
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

func createMacPlatformConfiguration() (*vz.MacPlatformConfiguration, error) {
	auxiliaryStorage, err := vz.NewMacAuxiliaryStorage(GetAuxiliaryStoragePath())
	if err != nil {
		return nil, fmt.Errorf("failed to create a new mac auxiliary storage: %w", err)
	}
	hardwareModel, err := vz.NewMacHardwareModelWithDataPath(
		GetHardwareModelPath(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new hardware model: %w", err)
	}
	machineIdentifier, err := vz.NewMacMachineIdentifierWithDataPath(
		GetMachineIdentifierPath(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new machine identifier: %w", err)
	}
	return vz.NewMacPlatformConfiguration(
		vz.WithAuxiliaryStorage(auxiliaryStorage),
		vz.WithHardwareModel(hardwareModel),
		vz.WithMachineIdentifier(machineIdentifier),
	), nil
}

func setupVMConfiguration(platformConfig vz.PlatformConfiguration) (*vz.VirtualMachineConfiguration, error) {
	config := vz.NewVirtualMachineConfiguration(
		vz.NewMacOSBootLoader(),
		computeCPUCount(),
		computeMemorySize(),
	)
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
