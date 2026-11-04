package vz

import "context"

func Available(version float64) bool {
	return macOSAvailable(version) != nil
}

func StartVirtualMachineContext(ctx context.Context, machine *VirtualMachine, opts ...VirtualMachineStartOption) error {
	return machine.start(ctx, opts...)
}

func StopVirtualMachineContext(ctx context.Context, machine *VirtualMachine) error {
	return machine.stop(ctx)
}
