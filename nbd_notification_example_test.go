//go:build darwin

package vz_test

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/Code-Hex/vz/v4"
)

// This example requires macOS 14 or newer, the network client entitlement, and an
// NBD server at 127.0.0.1:10809 serving a disk named "export".
func ExampleNetworkBlockDeviceStorageDeviceAttachment_WasConnectedNotify() {
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
	attachment, err := vz.NewNetworkBlockDeviceStorageDeviceAttachment(
		"nbd://127.0.0.1:10809/export", 5*time.Second, true, vz.DiskSynchronizationModeFull,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer runtime.KeepAlive(attachment)
	disk, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		fmt.Println(err)
		return
	}
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{disk})
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

	events := make(chan struct{}, 16)
	stop, err := attachment.WasConnectedNotify(events)
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
	case <-events:
		fmt.Println("NBD connected or reconnected")
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}

// Use a running NBD server at 127.0.0.1:10809 with a disk named "export".
// This example requires macOS 14 or newer and the network client entitlement.
// An error event means the NBD client can no longer function.
func ExampleNetworkBlockDeviceStorageDeviceAttachment_DidEncounterErrorNotify() {
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
	attachment, err := vz.NewNetworkBlockDeviceStorageDeviceAttachment(
		"nbd://127.0.0.1:10809/export", 5*time.Second, true, vz.DiskSynchronizationModeFull,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer runtime.KeepAlive(attachment)
	disk, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		fmt.Println(err)
		return
	}
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{disk})
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

	events := make(chan error, 16)
	stop, err := attachment.DidEncounterErrorNotify(events)
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
	case err := <-events:
		fmt.Println("unrecoverable NBD error:", err)
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}
