//go:build darwin

package vz_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Code-Hex/vz/v4"
)

// The guest must open or close the virtio port named "control" to produce an event.
// This example requires macOS 13 or newer and a guest with virtio console support.
func ExampleVirtualMachine_ConsolePortChangedNotify() {
	loader, err := vz.NewLinuxBootLoader("testdata/Image", vz.WithInitrd("testdata/initramfs.cpio.gz"))
	if err != nil {
		fmt.Println(err)
		return
	}
	config, err := vz.NewVirtualMachineConfiguration(loader, 1, 512*1024*1024)
	if err != nil {
		fmt.Println(err)
		return
	}
	console, err := vz.NewVirtioConsoleDeviceConfiguration()
	if err != nil {
		fmt.Println(err)
		return
	}
	port, err := vz.NewVirtioConsolePortConfiguration(vz.WithVirtioConsolePortConfigurationName("control"))
	if err != nil {
		fmt.Println(err)
		return
	}
	console.SetVirtioConsolePortConfiguration(1, port)
	config.SetConsoleDevicesVirtualMachineConfiguration([]vz.ConsoleDeviceConfiguration{console})
	machine, err := vz.NewVirtualMachine(config)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if machine.CanStop() {
			if err := machine.Stop(); err != nil {
				fmt.Println(err)
			}
		}
	}()

	events := make(chan vz.ConsolePortEvent, 16)
	stop, err := machine.ConsolePortChangedNotify(events)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer stop()
	if err := machine.Start(); err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case event := <-events:
		fmt.Printf("console %d port %d open=%t\n", event.DeviceIndex, event.PortIndex, event.Open)
		// Before talking to a new guest process, process or flush pending attachment data.
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}

// This example requires macOS 14 or newer and a guest with virtio graphics support.
func ExampleVirtualMachine_GraphicsDisplayChangedNotify() {
	loader, err := vz.NewLinuxBootLoader("testdata/Image", vz.WithInitrd("testdata/initramfs.cpio.gz"))
	if err != nil {
		fmt.Println(err)
		return
	}
	config, err := vz.NewVirtualMachineConfiguration(loader, 1, 512*1024*1024)
	if err != nil {
		fmt.Println(err)
		return
	}
	graphics, err := vz.NewVirtioGraphicsDeviceConfiguration()
	if err != nil {
		fmt.Println(err)
		return
	}
	scanout, err := vz.NewVirtioGraphicsScanoutConfiguration(800, 600)
	if err != nil {
		fmt.Println(err)
		return
	}
	graphics.SetScanouts(scanout)
	config.SetGraphicsDevicesVirtualMachineConfiguration([]vz.GraphicsDeviceConfiguration{graphics})
	machine, err := vz.NewVirtualMachine(config)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if machine.CanStop() {
			if err := machine.Stop(); err != nil {
				fmt.Println(err)
			}
		}
	}()

	events := make(chan vz.GraphicsDisplayEvent, 16)
	stop, err := machine.GraphicsDisplayChangedNotify(events)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer stop()
	if err := machine.Start(); err != nil {
		fmt.Println(err)
		return
	}

	display := machine.GraphicsDevices()[0].Displays()[0]
	if err := display.ReconfigureWithSizeInPixels(1024, 768); err != nil {
		fmt.Println(err)
		return
	}
	// The guest may ignore the request, and either reconfiguration event may be dropped.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case event := <-events:
		fmt.Printf("graphics %d display %d reconfiguring=%t\n", event.DeviceIndex, event.DisplayIndex, event.Reconfiguring)
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}
