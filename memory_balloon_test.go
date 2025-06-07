package vz_test

import (
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3"
)

func TestVirtioTraditionalMemoryBalloonDeviceConfiguration(t *testing.T) {
	// Create memory balloon device configuration
	config, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		t.Fatalf("failed to create memory balloon device configuration: %v", err)
	}
	if config == nil {
		t.Fatal("memory balloon configuration should not be nil")
	}
}

func TestMemoryBalloonDevices(t *testing.T) {
	// Create a simple VM configuration
	bootLoader, err := vz.NewLinuxBootLoader(
		"./testdata/Image",
		vz.WithCommandLine("console=hvc0"),
	)
	if err != nil {
		t.Fatalf("failed to create boot loader: %v", err)
	}

	config, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		1,
		256*1024*1024,
	)
	if err != nil {
		t.Fatalf("failed to create virtual machine configuration: %v", err)
	}

	// Create and add a memory balloon device
	memoryBalloonConfig, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		t.Fatalf("failed to create memory balloon device configuration: %v", err)
	}

	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{
		memoryBalloonConfig,
	})

	// Create the VM
	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		t.Fatalf("failed to create virtual machine: %v", err)
	}

	// Get memory balloon devices
	balloonDevices := vm.MemoryBalloonDevices()
	if len(balloonDevices) != 1 {
		t.Fatalf("expected 1 memory balloon device, got %d", len(balloonDevices))
	}

	// Verify we can access the balloon device
	balloonDevice := balloonDevices[0]
	if balloonDevice == nil {
		t.Fatal("memory balloon device should not be nil")
	}

	// Verify we can cast to VirtioTraditionalMemoryBalloonDevice
	traditionalDevice := vz.AsVirtioTraditionalMemoryBalloonDevice(balloonDevice)
	if traditionalDevice == nil {
		t.Fatal("failed to cast to VirtioTraditionalMemoryBalloonDevice")
	}
}

func TestMemoryBalloonTargetSizeAdjustment(t *testing.T) {
	// Create a VM with a memory balloon device
	bootLoader, err := vz.NewLinuxBootLoader(
		"./testdata/Image",
		vz.WithCommandLine("console=hvc0"),
		vz.WithInitrd("./testdata/initramfs.cpio.gz"),
	)
	if err != nil {
		t.Fatalf("failed to create boot loader: %v", err)
	}

	startingMemory := uint64(512 * 1024 * 1024)
	targetMemory := uint64(300 * 1024 * 1024)

	t.Logf("Starting memory: %d bytes", startingMemory)
	t.Logf("Target memory:   %d bytes", targetMemory)

	config, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		1,
		startingMemory,
	)
	if err != nil {
		t.Fatalf("failed to create virtual machine configuration: %v", err)
	}

	// Create memory balloon device
	memoryBalloonConfig, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		t.Fatalf("failed to create memory balloon device configuration: %v", err)
	}

	// Add memory balloon device to VM configuration
	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{
		memoryBalloonConfig,
	})

	// Validate the configuration
	valid, err := config.Validate()
	if err != nil {
		t.Fatalf("configuration validation failed: %v", err)
	}
	if !valid {
		t.Fatal("configuration is not valid")
	}

	// Create the VM
	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		t.Fatalf("failed to create virtual machine: %v", err)
	}

	// Check memory balloon devices
	balloonDevices := vm.MemoryBalloonDevices()
	if len(balloonDevices) != 1 {
		t.Fatalf("expected 1 memory balloon device, got %d", len(balloonDevices))
	}

	// Cast to VirtioTraditionalMemoryBalloonDevice
	balloonDevice := vz.AsVirtioTraditionalMemoryBalloonDevice(balloonDevices[0])
	if balloonDevice == nil {
		t.Fatal("failed to cast to VirtioTraditionalMemoryBalloonDevice")
	}

	// Start the VM
	t.Log("Starting virtual machine...")
	err = vm.Start()
	if err != nil {
		t.Fatalf("failed to start virtual machine: %v", err)
	}

	defer func() {
		if vm.CanStop() {
			_ = vm.Stop() // Cleanup VM
		}
	}()

	// Wait until the VM is running
	err = waitUntilState(10*time.Second, vm, vz.VirtualMachineStateRunning)
	if err != nil {
		t.Fatalf("failed to wait for VM to start: %v", err)
	}

	// Get the current target memory size
	currentMemoryBefore := balloonDevice.GetTargetVirtualMachineMemorySize()

	if currentMemoryBefore != startingMemory {
		t.Fatalf("expected starting memory size to be %d, got %d", startingMemory, currentMemoryBefore)
	}

	// Set a new target memory size
	balloonDevice.SetTargetVirtualMachineMemorySize(targetMemory)

	// Verify the new memory size was set
	currentMemoryAfter := balloonDevice.GetTargetVirtualMachineMemorySize()

	if currentMemoryAfter != targetMemory {
		t.Fatalf("expected memory size after adjustment to be %d, got %d", targetMemory, currentMemoryAfter)
	}
}
