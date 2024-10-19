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
var nbdURL string

func init() {
	flag.BoolVar(&install, "install", false, "run command as install mode")
	flag.StringVar(&nbdURL, "nbd-url", "", "nbd url (e.g. nbd+unix:///export?socket=nbd.sock)")
}

func main() {
	flag.Parse()
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if install {
		return installMacOS(ctx)
	}
	return runVM(ctx)
}

func runVM(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	platformConfig, err := createMacPlatformConfiguration()
	if err != nil {
		return err
	}
	config, err := setupVMConfiguration(platformConfig)
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
					return
				}
				// if err := vm.Pause(); err != nil {
				// 	log.Println("pause with error", err)
				// 	return
				// }
				// if err := vm.SaveMachineStateToPath("savestate"); err != nil {
				// 	log.Println("save state with error", err)
				// }
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
	var attachment vz.StorageDeviceAttachment
	var err error

	if nbdURL == "" {
		// create disk image with 64 GiB
		if err := vz.CreateDiskImage(diskPath, 64*1024*1024*1024); err != nil {
			if !os.IsExist(err) {
				return nil, fmt.Errorf("failed to create disk image: %w", err)
			}
		}

		attachment, err = vz.NewDiskImageStorageDeviceAttachment(
			diskPath,
			false,
		)
	} else {
		attachment, err = vz.NewNetworkBlockDeviceStorageDeviceAttachment(
			nbdURL,
			10*time.Second,
			false,
			vz.DiskSynchronizationModeFull,
		)
	}
	if err != nil {
		return nil, err
	}
	return vz.NewVirtioBlockDeviceConfiguration(attachment)
}

func createGraphicsDeviceConfiguration() (*vz.MacGraphicsDeviceConfiguration, error) {
	graphicDeviceConfig, err := vz.NewMacGraphicsDeviceConfiguration()
	if err != nil {
		return nil, err
	}
	graphicsDisplayConfig, err := vz.NewMacGraphicsDisplayConfiguration(1920, 1200, 80)
	if err != nil {
		return nil, err
	}
	graphicDeviceConfig.SetDisplays(
		graphicsDisplayConfig,
	)
	return graphicDeviceConfig, nil
}

func createNetworkDeviceConfiguration() (*vz.VirtioNetworkDeviceConfiguration, error) {
	natAttachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return nil, err
	}
	return vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
}

func createKeyboardConfiguration() (vz.KeyboardConfiguration, error) {
	config, err := vz.NewMacKeyboardConfiguration()
	if err != nil {
		if errors.Is(err, vz.ErrUnsupportedOSVersion) {
			return vz.NewUSBKeyboardConfiguration()
		}
		return nil, err
	}
	return config, nil
}

func createAudioDeviceConfiguration() (*vz.VirtioSoundDeviceConfiguration, error) {
	audioConfig, err := vz.NewVirtioSoundDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create sound device configuration: %w", err)
	}
	inputStream, err := vz.NewVirtioSoundDeviceHostInputStreamConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create input stream configuration: %w", err)
	}
	outputStream, err := vz.NewVirtioSoundDeviceHostOutputStreamConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create output stream configuration: %w", err)
	}
	audioConfig.SetStreams(
		inputStream,
		outputStream,
	)
	return audioConfig, nil
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
		vz.WithMacAuxiliaryStorage(auxiliaryStorage),
		vz.WithMacHardwareModel(hardwareModel),
		vz.WithMacMachineIdentifier(machineIdentifier),
	)
}

func setupVMConfiguration(platformConfig vz.PlatformConfiguration) (*vz.VirtualMachineConfiguration, error) {
	bootloader, err := vz.NewMacOSBootLoader()
	if err != nil {
		return nil, err
	}

	config, err := vz.NewVirtualMachineConfiguration(
		bootloader,
		computeCPUCount(),
		computeMemorySize(),
	)
	if err != nil {
		return nil, err
	}
	config.SetPlatformVirtualMachineConfiguration(platformConfig)
	graphicsDeviceConfig, err := createGraphicsDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create graphics device configuration: %w", err)
	}
	config.SetGraphicsDevicesVirtualMachineConfiguration([]vz.GraphicsDeviceConfiguration{
		graphicsDeviceConfig,
	})
	blockDeviceConfig, err := createBlockDeviceConfiguration(GetDiskImagePath())
	if err != nil {
		return nil, fmt.Errorf("failed to create block device configuration: %w", err)
	}
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{blockDeviceConfig})

	networkDeviceConfig, err := createNetworkDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create network device configuration: %w", err)
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{
		networkDeviceConfig,
	})

	usbScreenPointingDevice, err := vz.NewUSBScreenCoordinatePointingDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create pointing device configuration: %w", err)
	}
	pointingDevices := []vz.PointingDeviceConfiguration{usbScreenPointingDevice}

	trackpad, err := vz.NewMacTrackpadConfiguration()
	if err == nil {
		pointingDevices = append(pointingDevices, trackpad)
	}
	config.SetPointingDevicesVirtualMachineConfiguration(pointingDevices)

	keyboardDeviceConfig, err := createKeyboardConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create keyboard device configuration: %w", err)
	}
	config.SetKeyboardsVirtualMachineConfiguration([]vz.KeyboardConfiguration{
		keyboardDeviceConfig,
	})

	audioDeviceConfig, err := createAudioDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create audio device configuration: %w", err)
	}
	config.SetAudioDevicesVirtualMachineConfiguration([]vz.AudioDeviceConfiguration{
		audioDeviceConfig,
	})

	validated, err := config.Validate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate configuration: %w", err)
	}
	if !validated {
		return nil, fmt.Errorf("invalid configuration")
	}

	// If you want to try this one, you need to comment out a few of configs.
	//
	// if _, err := config.ValidateSaveRestoreSupport(); err != nil {
	// 	return nil, fmt.Errorf("failed to validate save restore configuration: %w", err)
	// }

	return config, nil
}
