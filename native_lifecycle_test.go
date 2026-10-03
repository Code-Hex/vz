//go:build darwin

package vz

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestNativeLateCallbackReleasesPayloads(t *testing.T) {
	for _, kind := range []uint32{1, 3, 5, 6, 7, 9, 10, 11} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			pool := pureobjc.ID(pureobjc.GetClass("NSAutoreleasePool")).Send(pureobjc.RegisterName("new"))
			allocated := pureobjc.ID(pureobjc.GetClass("NSHashTable")).Send(pureobjc.RegisterName("alloc"))
			const weakMemory = uintptr(5)
			weak := allocated.Send(pureobjc.RegisterName("initWithOptions:capacity:"), weakMemory, uintptr(2))
			defer weak.Send(pureobjc.RegisterName("release"))
			first := pureobjc.Send[unsafe.Pointer](pureobjc.ID(pureobjc.GetClass("NSObject")), pureobjc.RegisterName("new"))
			weak.Send(pureobjc.RegisterName("addObject:"), first)
			var second unsafe.Pointer
			if kind == 5 || kind == 10 {
				second = pureobjc.Send[unsafe.Pointer](pureobjc.ID(pureobjc.GetClass("NSObject")), pureobjc.RegisterName("new"))
				weak.Send(pureobjc.RegisterName("addObject:"), second)
			}
			pool.Send(pureobjc.RegisterName("release"))
			dispatchNativeCallback(kind, 0, first, second, 0)
			vzbridge.Drain()
			if got := weak.Send(pureobjc.RegisterName("anyObject")); got != 0 {
				t.Fatalf("late callback kind %d retained a payload", kind)
			}
		})
	}
}

func TestNativeRuntimeDeviceKeepsMachineAlive(t *testing.T) {
	device, states := retainedBalloonDevice(t)
	for range 3 {
		runtime.GC()
		vzbridge.Drain()
	}
	select {
	case _, open := <-states:
		if !open {
			t.Fatal("machine was released while its runtime device remained live")
		}
	default:
	}
	device.SetTargetVirtualMachineMemorySize(512 * 1024 * 1024)
	if got := device.GetTargetVirtualMachineMemorySize(); got != 512*1024*1024 {
		t.Fatalf("balloon target = %d", got)
	}
	runtime.KeepAlive(device)
	device = nil
	awaitNativeBridge(t, func() bool {
		select {
		case _, open := <-states:
			return !open
		default:
			return false
		}
	})
}

func retainedBalloonDevice(t *testing.T) (*VirtioTraditionalMemoryBalloonDevice, <-chan VirtualMachineState) {
	t.Helper()
	config := newTestConfig(t)
	balloon, err := NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]MemoryBalloonDeviceConfiguration{balloon})
	machine, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	devices := machine.MemoryBalloonDevices()
	if len(devices) != 1 {
		t.Fatalf("balloon count = %d", len(devices))
	}
	return devices[0].(*VirtioTraditionalMemoryBalloonDevice), machine.StateChangedNotify()
}

func TestNativeConcurrentMachines(t *testing.T) {
	if err := macOSAvailable(12); err != nil {
		t.Skip("stopping a virtual machine requires macOS 12")
	}
	machines := make([]*VirtualMachine, 2)
	for i := range machines {
		loader, err := NewLinuxBootLoader("testdata/Image", WithInitrd("testdata/initramfs.cpio.gz"))
		if err != nil {
			t.Fatal(err)
		}
		config, err := NewVirtualMachineConfiguration(loader, 1, 256*1024*1024)
		if err != nil {
			t.Fatal(err)
		}
		machines[i], err = NewVirtualMachine(config)
		if err != nil {
			t.Fatal(err)
		}
		machine := machines[i]
		t.Cleanup(func() {
			if machine.CanStop() {
				if err := machine.Stop(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	completed := make(chan error, len(machines))
	for _, machine := range machines {
		go func() {
			for _, operation := range []func() error{func() error { return machine.Start() }, machine.Pause, machine.Resume, machine.Stop} {
				if err := operation(); err != nil {
					completed <- err
					return
				}
			}
			completed <- nil
		}()
	}
	for range machines {
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("concurrent machine operations did not complete")
		}
	}
}
