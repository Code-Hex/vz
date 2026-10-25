//go:build darwin

package vz_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Code-Hex/vz/v4"
)

// This example requires macOS 12 or newer. An event reports a framework attachment
// failure, not a guest network outage or an unreachable remote host.
func ExampleVirtualMachine_NetworkDeviceAttachmentWasDisconnectedNotify() {
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
	attachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		fmt.Println(err)
		return
	}
	network, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
	if err != nil {
		fmt.Println(err)
		return
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{network})
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

	events := make(chan *vz.DisconnectedError, 16)
	stop, err := machine.NetworkDeviceAttachmentWasDisconnectedNotify(events)
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
		fmt.Println("network attachment disconnected:", event.Err)
		if event.Config == network {
			fmt.Println("the configured NAT device was affected")
		}
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}
