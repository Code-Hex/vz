package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/Code-Hex/vz/v3"
)

var install bool

func init() {
	flag.BoolVar(&install, "install", false, "run command as install mode")
}

func main() {
	flag.Parse()
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	installerISOPath := os.Getenv("INSTALLER_ISO_PATH")

	if install {
		if installerISOPath == "" {
			return fmt.Errorf("must be specified INSTALLER_ISO_PATH env")
		}
		if err := CreateVMBundle(); err != nil {
			return err
		}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	config, err := createVirtualMachineConfig(installerISOPath, install)
	if err != nil {
		return err
	}

	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		return err
	}

	if err := vm.Start(); err != nil {
		return err
	}

	errCh := make(chan error, 1)

	go func() {
		for {
			select {
			case newState := <-vm.StateChangedNotify():
				if newState == vz.VirtualMachineStateRunning {
					log.Println("start VM is running")
				}
				if newState == vz.VirtualMachineStateStopped || newState == vz.VirtualMachineStateStopping {
					log.Println("stopped state")
					errCh <- nil
					return
				}
			case err := <-errCh:
				errCh <- fmt.Errorf("failed to start vm: %w", err)
				return
			}
		}
	}()

	go func() {
		if !vm.CanStop() {
			log.Println("cannot stop vm forcefully")
			return
		}
		time.Sleep(10 * time.Second)
		log.Println("calling vm.Stop()")

		vm.Stop()
	}()

	// cleanup is this function is useful when finished graphic application.
	cleanup := func() {
		for i := 1; vm.CanRequestStop(); i++ {
			result, err := vm.RequestStop()
			log.Printf("sent stop request(%d): %t, %v", i, result, err)
			time.Sleep(time.Second * 3)
			if i > 3 {
				log.Println("call stop")
				if err := vm.Stop(); err != nil {
					log.Println("stop with error", err)
				}
			}
		}
		log.Println("finished cleanup")
	}

	runtime.LockOSThread()
	vm.StartGraphicApplication(960, 600)
	runtime.UnlockOSThread()

	cleanup()

	return <-errCh
}

// Create an empty disk image for the virtual machine.
func createMainDiskImage(diskPath string) error {
	// create disk image with 64 GiB
	if err := vz.CreateDiskImage(diskPath, 64*1024*1024*1024); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("failed to create disk image: %w", err)
		}
	}
	return nil
}

func createBlockDeviceConfiguration(diskPath string) (*vz.VirtioBlockDeviceConfiguration, error) {
	attachment, err := vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync(diskPath, false, vz.DiskImageCachingModeAutomatic, vz.DiskImageSynchronizationModeFsync)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new disk image storage device attachment: %w", err)
	}
	mainDisk, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new block deveice config: %w", err)
	}
	return mainDisk, nil
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

func createAndSaveMachineIdentifier(identifierPath string) (*vz.GenericMachineIdentifier, error) {
	machineIdentifier, err := vz.NewGenericMachineIdentifier()
	if err != nil {
		return nil, fmt.Errorf("failed to create a new machine identifier: %w", err)
	}
	err = CreateFileAndWriteTo(machineIdentifier.DataRepresentation(), identifierPath)
	if err != nil {
		return nil, fmt.Errorf("failed to save machine identifier data: %w", err)
	}
	return machineIdentifier, nil
}

func createEFIVariableStore(efiVariableStorePath string) (*vz.EFIVariableStore, error) {
	variableStore, err := vz.NewEFIVariableStore(
		efiVariableStorePath,
		vz.WithCreatingEFIVariableStore(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create EFI variable store: %w", err)
	}
	return variableStore, nil
}

func createUSBMassStorageDeviceConfiguration(installerISOPath string) (*vz.USBMassStorageDeviceConfiguration, error) {
	installerDiskAttachment, err := vz.NewDiskImageStorageDeviceAttachment(
		installerISOPath,
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new disk attachment for USBMassConfiguration: %w", err)
	}
	config, err := vz.NewUSBMassStorageDeviceConfiguration(installerDiskAttachment)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new USB storage device: %w", err)
	}
	return config, nil
}

func createNetworkDeviceConfiguration() (*vz.VirtioNetworkDeviceConfiguration, error) {
	natAttachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return nil, fmt.Errorf("nat attachment initialization failed: %w", err)
	}
	netConfig, err := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	if err != nil {
		return nil, fmt.Errorf("failed to create a network device: %w", err)
	}
	return netConfig, nil
}

func createGraphicsDeviceConfiguration() (*vz.VirtioGraphicsDeviceConfiguration, error) {
	graphicDeviceConfig, err := vz.NewVirtioGraphicsDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize virtio graphic device: %w", err)
	}
	graphicsScanoutConfig, err := vz.NewVirtioGraphicsScanoutConfiguration(1920, 1200)
	if err != nil {
		return nil, fmt.Errorf("failed to create graphics scanout: %w", err)
	}
	graphicDeviceConfig.SetScanouts(
		graphicsScanoutConfig,
	)
	return graphicDeviceConfig, nil
}

func createInputAudioDeviceConfiguration() (*vz.VirtioSoundDeviceConfiguration, error) {
	audioConfig, err := vz.NewVirtioSoundDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create sound device configuration: %w", err)
	}
	inputStream, err := vz.NewVirtioSoundDeviceHostInputStreamConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create input stream configuration: %w", err)
	}
	audioConfig.SetStreams(
		inputStream,
	)
	return audioConfig, nil
}

func createOutputAudioDeviceConfiguration() (*vz.VirtioSoundDeviceConfiguration, error) {
	audioConfig, err := vz.NewVirtioSoundDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create sound device configuration: %w", err)
	}
	outputStream, err := vz.NewVirtioSoundDeviceHostOutputStreamConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create output stream configuration: %w", err)
	}
	audioConfig.SetStreams(
		outputStream,
	)
	return audioConfig, nil
}

func createSpiceAgentConsoleDeviceConfiguration() (*vz.VirtioConsoleDeviceConfiguration, error) {
	consoleDevice, err := vz.NewVirtioConsoleDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create a new console device: %w", err)
	}

	spiceAgentAttachment, err := vz.NewSpiceAgentPortAttachment()
	if err != nil {
		return nil, fmt.Errorf("failed to create a new spice agent attachment: %w", err)
	}
	spiceAgentName, err := vz.SpiceAgentPortAttachmentName()
	if err != nil {
		return nil, fmt.Errorf("failed to get spice agent name: %w", err)
	}
	spiceAgentPort, err := vz.NewVirtioConsolePortConfiguration(
		vz.WithVirtioConsolePortConfigurationAttachment(spiceAgentAttachment),
		vz.WithVirtioConsolePortConfigurationName(spiceAgentName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new console port for spice agent: %w", err)
	}

	consoleDevice.SetVirtioConsolePortConfiguration(0, spiceAgentPort)

	return consoleDevice, nil
}

func getMachineIdentifier(needsInstall bool) (*vz.GenericMachineIdentifier, error) {
	path := GetMachineIdentifierPath()
	if needsInstall {
		return createAndSaveMachineIdentifier(path)
	}
	return vz.NewGenericMachineIdentifierWithDataPath(path)
}

func getEFIVariableStore(needsInstall bool) (*vz.EFIVariableStore, error) {
	path := GetEFIVariableStorePath()
	if needsInstall {
		return createEFIVariableStore(path)
	}
	return vz.NewEFIVariableStore(path)
}

func createVirtualMachineConfig(installerISOPath string, needsInstall bool) (*vz.VirtualMachineConfiguration, error) {
	machineIdentifier, err := getMachineIdentifier(needsInstall)
	if err != nil {
		return nil, err
	}
	platformConfig, err := vz.NewGenericPlatformConfiguration(
		vz.WithGenericMachineIdentifier(machineIdentifier),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new platform config: %w", err)
	}

	efiVariableStore, err := getEFIVariableStore(needsInstall)
	if err != nil {
		return nil, err
	}

	bootLoader, err := vz.NewEFIBootLoader(
		vz.WithEFIVariableStore(efiVariableStore),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new EFI boot loader: %w", err)
	}

	disks := make([]vz.StorageDeviceConfiguration, 0)
	if needsInstall {
		usbConfig, err := createUSBMassStorageDeviceConfiguration(installerISOPath)
		if err != nil {
			return nil, err
		}
		disks = append(disks, usbConfig)
	}

	config, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		computeCPUCount(),
		computeMemorySize(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create vm config: %w", err)
	}

	config.SetPlatformVirtualMachineConfiguration(platformConfig)

	// Set graphic device
	graphicsDeviceConfig, err := createGraphicsDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create graphics device configuration: %w", err)
	}
	config.SetGraphicsDevicesVirtualMachineConfiguration([]vz.GraphicsDeviceConfiguration{
		graphicsDeviceConfig,
	})

	// Set storage device
	mainDiskPath := GetMainDiskImagePath()
	if needsInstall {
		if err := createMainDiskImage(mainDiskPath); err != nil {
			return nil, fmt.Errorf("failed to create a main disk image: %w", err)
		}
	}
	mainDisk, err := createBlockDeviceConfiguration(mainDiskPath)
	if err != nil {
		return nil, err
	}
	disks = append(disks, mainDisk)
	config.SetStorageDevicesVirtualMachineConfiguration(disks)

	consoleDeviceConfig, err := createSpiceAgentConsoleDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create console device configuration: %w", err)
	}
	config.SetConsoleDevicesVirtualMachineConfiguration([]vz.ConsoleDeviceConfiguration{
		consoleDeviceConfig,
	})

	// Set network device
	networkDeviceConfig, err := createNetworkDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create network device configuration: %w", err)
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{
		networkDeviceConfig,
	})

	// Set audio device
	inputAudioDeviceConfig, err := createInputAudioDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create input audio device configuration: %w", err)
	}
	outputAudioDeviceConfig, err := createOutputAudioDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create output audio device configuration: %w", err)
	}
	config.SetAudioDevicesVirtualMachineConfiguration([]vz.AudioDeviceConfiguration{
		inputAudioDeviceConfig,
		outputAudioDeviceConfig,
	})

	// Set pointing device
	pointingDeviceConfig, err := vz.NewUSBScreenCoordinatePointingDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create pointing device configuration: %w", err)
	}
	config.SetPointingDevicesVirtualMachineConfiguration([]vz.PointingDeviceConfiguration{
		pointingDeviceConfig,
	})

	// Set keyboard device
	keyboardDeviceConfig, err := vz.NewUSBKeyboardConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create keyboard device configuration: %w", err)
	}
	config.SetKeyboardsVirtualMachineConfiguration([]vz.KeyboardConfiguration{
		keyboardDeviceConfig,
	})

	// Set rosetta directory share
	directorySharingConfigs := make([]vz.DirectorySharingDeviceConfiguration, 0)
	directorySharingDeviceConfig, err := createRosettaDirectoryShareConfiguration()
	if err != nil && !errors.Is(err, errIgnoreInstall) {
		return nil, err
	}
	if directorySharingDeviceConfig != nil {
		directorySharingConfigs = append(directorySharingConfigs, directorySharingDeviceConfig)
	}

	config.SetDirectorySharingDevicesVirtualMachineConfiguration(directorySharingConfigs)

	validated, err := config.Validate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate configuration: %w", err)
	}
	if !validated {
		return nil, fmt.Errorf("invalid configuration")
	}

	return config, nil
}
