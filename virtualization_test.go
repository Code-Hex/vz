package vz

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type testVM struct {
	*VirtualMachine
	tempKernelFile    *os.File
	stateHandlerError func(err error)
}

func (vm *testVM) Close() error {
	_ = os.Remove(vm.tempKernelFile.Name())
	return vm.tempKernelFile.Close()
}

func newTestVM(t *testing.T) *testVM {
	// use empty file as dummy kernel as we don't expect the VM to successfully start in our tests
	tempKernelFile, err := os.CreateTemp(".", "vz_vmlinuz_test")
	require.NoError(t, err)
	bootloader := NewLinuxBootLoader(tempKernelFile.Name())
	config := NewVirtualMachineConfiguration(bootloader, 1, 64*1024*1024)
	//passing the config below to NewVirtualMachine reproduces https://github.com/Code-Hex/vz/issues/43
	//config := NewVirtualMachineConfiguration(&LinuxBootLoader{}, 1, 64*1024*1024)

	stateHandlerError := func(err error) {
		require.Error(t, err)
	}

	return &testVM{
		VirtualMachine:    NewVirtualMachine(config),
		tempKernelFile:    tempKernelFile,
		stateHandlerError: stateHandlerError,
	}
}

func TestStart(t *testing.T) {
	vm := newTestVM(t)
	require.NotEqual(t, vm, nil)
	defer vm.Close()

	require.True(t, vm.CanStart())
	vm.Start(vm.stateHandlerError)
}

func TestPause(t *testing.T) {
	vm := newTestVM(t)
	require.NotEqual(t, vm, nil)
	defer vm.Close()

	require.False(t, vm.CanPause())
	vm.Pause(vm.stateHandlerError)
}

func TestResume(t *testing.T) {
	vm := newTestVM(t)
	require.NotEqual(t, vm, nil)
	defer vm.Close()

	require.False(t, vm.CanResume())
	vm.Resume(vm.stateHandlerError)
}

func TestRequestStop(t *testing.T) {
	vm := newTestVM(t)
	require.NotEqual(t, vm, nil)
	defer vm.Close()

	require.False(t, vm.CanRequestStop())
	_, err := vm.RequestStop()
	require.Error(t, err)
}
