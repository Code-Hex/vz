//go:build darwin

package vz_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func ExampleVirtualMachine_StateChangedNotify() {
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

	changed := make(chan vz.VirtualMachineState, 1)
	stop, err := machine.StateChangedNotify(changed)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer stop()
	if err := machine.Start(); err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Notifications can be dropped. Read the current state before each wait.
	for machine.State() != vz.VirtualMachineStateRunning {
		select {
		case <-changed:
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		}
	}
	fmt.Println("running")
	// Output: running
}
