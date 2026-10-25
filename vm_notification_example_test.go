//go:build darwin

package vz_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Code-Hex/vz/v4"
)

// This example needs a guest that handles the shutdown request. The host's Stop
// method does not produce a GuestDidStopNotify notification.
func ExampleVirtualMachine_GuestDidStopNotify() {
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
	stop, err := machine.GuestDidStopNotify(events)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer stop()
	if err := machine.Start(); err != nil {
		fmt.Println(err)
		return
	}

	accepted, err := machine.RequestStop()
	if err != nil {
		fmt.Println(err)
		return
	}
	if !accepted {
		fmt.Println("guest shutdown request was not accepted")
		return
	}
	// Acceptance does not guarantee shutdown. Even a shutdown notification can be dropped.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case <-events:
		fmt.Println("guest shut down")
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}

// This example observes a running VM. An error notification arrives only if the
// framework stops the VM because of an error; a guest application failure is not enough.
func ExampleVirtualMachine_DidStopWithErrorNotify() {
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
	stop, err := machine.DidStopWithErrorNotify(events)
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
		fmt.Println("VM stopped:", err)
	case <-ctx.Done():
		fmt.Println(ctx.Err())
	}
}
