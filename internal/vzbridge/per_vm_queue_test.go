//go:build darwin

package vzbridge

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/ebitengine/purego"
)

func newQueueTestMachine(t *testing.T) *objc.Pointer {
	t.Helper()
	if !sdkVersionAvailable(sdkOSVersion, "13.0", "") {
		t.Skip("EFI boot loader requires macOS 13")
	}
	loader := VZEFIBootLoader_Init()
	path := NSString_StringWithUTF8String(t.TempDir() + "/efi-vars")
	url := NSURL_FileURLWithPath(path)
	var failure unsafe.Pointer
	store := VZEFIVariableStore_InitCreatingVariableStoreAtURL_Options_Error(url, 1, &failure)
	if store == nil {
		t.Fatal("EFI variable store creation failed")
	}
	VZEFIBootLoader_SetVariableStore(loader, store)
	config := NewVZVirtualMachineConfiguration(loader, 1, 512*1024*1024)
	socket := VZVirtioSocketDeviceConfiguration_Init()
	devices := NSMutableArray_New()
	NSMutableArray_AddObject(devices, socket)
	VZVirtualMachineConfiguration_SetSocketDevices(config, devices)
	if !VZVirtualMachineConfiguration_ValidateWithError(config, &failure) {
		if failure != nil {
			ReleaseObject(failure)
		}
		t.Fatal("queue test configuration is invalid")
	}
	machine := NewVZVirtualMachineWithDispatchQueue(config, 0, 0)
	if machine == nil {
		t.Fatal("queue test machine creation failed")
	}
	for _, object := range []*objc.Pointer{config, loader, socket, devices} {
		if ObjectQueue(object) != DispatchQueuePointer() {
			t.Error("creating a machine assigned its queue to a configuration object")
		}
	}
	t.Cleanup(func() {
		for _, object := range []*objc.Pointer{machine, devices, socket, config, loader, store, url, path} {
			objc.Release(object)
		}
		Drain()
	})
	return machine
}

func TestVirtualMachinesUseDifferentNativeQueues(t *testing.T) {
	if !sdkVersionAvailable(sdkOSVersion, "26.0", "") {
		t.Skip("the native queue property requires macOS 26")
	}
	first := newQueueTestMachine(t)
	second := newQueueTestMachine(t)
	firstQueue := VZVirtualMachine_Queue(first)
	secondQueue := VZVirtualMachine_Queue(second)
	defer objc.Release(firstQueue)
	defer objc.Release(secondQueue)
	if objc.Ptr(firstQueue) == nil || objc.Ptr(secondQueue) == nil {
		t.Fatal("a machine has no native queue")
	}
	if ObjectQueue(firstQueue) != DispatchQueuePointer() || ObjectQueue(secondQueue) != DispatchQueuePointer() {
		t.Fatal("a native dispatch queue inherited its machine owner")
	}
	if objc.Ptr(firstQueue) == objc.Ptr(secondQueue) {
		t.Fatal("independent virtual machines share a native queue")
	}
}

func TestVirtualMachineQueuesRunIndependently(t *testing.T) {
	first := newQueueTestMachine(t)
	second := newQueueTestMachine(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	go func() {
		onObjectQueue(objc.Ptr(first), func() {
			close(entered)
			<-release
		})
		close(finished)
	}()
	awaitQueueTest(t, entered)
	otherState := make(chan int64, 1)
	go func() { otherState <- VZVirtualMachine_State(second) }()
	select {
	case state := <-otherState:
		if state != 0 {
			t.Errorf("new virtual machine state = %d, want stopped (0)", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("another virtual machine was blocked by the first machine's queue")
	}
	common := make(chan struct{})
	go func() {
		onQueue(func() {})
		close(common)
	}()
	awaitQueueTest(t, common)
	firstState := make(chan int64, 1)
	go func() { firstState <- VZVirtualMachine_State(first) }()
	select {
	case <-firstState:
		t.Error("the machine getter bypassed its blocked queue")
	case <-time.After(100 * time.Millisecond):
	}
	unblock.Do(func() { close(release) })
	awaitQueueTest(t, finished)
	select {
	case state := <-firstState:
		if state != 0 {
			t.Errorf("new virtual machine state = %d, want stopped (0)", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the machine getter did not resume")
	}
}

func awaitQueueTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("queue operation did not finish")
	}
}

func TestVirtualMachineQueueSerializesAndReenters(t *testing.T) {
	machine := newQueueTestMachine(t)
	var count int
	var workers sync.WaitGroup
	for range 12 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 20 {
				onObjectQueue(objc.Ptr(machine), func() {
					onObjectQueue(objc.Ptr(machine), func() { count++ })
					if state := VZVirtualMachine_State(machine); state != 0 {
						t.Errorf("new virtual machine state = %d, want stopped (0)", state)
					}
				})
			}
		}()
	}
	workers.Wait()
	if count != 240 {
		t.Fatalf("serialized updates = %d, want 240", count)
	}
}

func TestVirtualMachineSocketObjectsKeepTheirQueue(t *testing.T) {
	machine := newQueueTestMachine(t)
	devices := VZVirtualMachine_SocketDevices(machine)
	defer objc.Release(devices)
	if ObjectQueue(devices) != DispatchQueuePointer() {
		t.Fatal("the device array inherited its machine's queue")
	}
	description := NSObject_Description(machine)
	defer objc.Release(description)
	if ObjectQueue(description) != DispatchQueuePointer() {
		t.Fatal("the machine description inherited its machine's queue")
	}
	if count := NSArray_Count(devices); count != 1 {
		t.Fatalf("socket device count = %d, want 1", count)
	}
	device := NSArray_ObjectAtIndex(devices, 0)
	defer objc.Release(device)
	queue := ObjectQueue(machine)
	if queue == nil || queue == DispatchQueuePointer() {
		t.Fatal("machine does not own a dedicated queue")
	}
	if ObjectQueue(device) != queue {
		t.Fatal("socket device did not inherit its machine's queue")
	}
	retained := RetainObject(device)
	defer objc.Release(retained)
	if ObjectQueue(retained) != queue {
		t.Fatal("retaining a socket device lost its queue")
	}
	callbackDone := make(chan struct{}, 1)
	callback := purego.NewCallback(func(kind uint32, context uint64, first, second unsafe.Pointer, value uint64) uint64 {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if kind == CallbackContextReleased && context == 901 {
			if state := VZVirtualMachine_State(machine); state != 0 {
				t.Errorf("callback machine state = %d, want stopped (0)", state)
			}
			callbackDone <- struct{}{}
		}
		return 0
	})
	SetCallback(uint64(callback))
	defer SetCallback(0)
	listener := NewVZVirtioSocketListener(device, 901)
	defer objc.Release(listener)
	VZVirtioSocketDevice_SetSocketListener_ForPort(device, listener, 1024)
	if ObjectQueue(listener) != queue {
		t.Fatal("socket listener did not inherit its device's queue")
	}
	VZVirtioSocketDevice_RemoveSocketListenerForPort(device, 1024)
	InvalidateVZVirtioSocketListener(listener)
	awaitQueueTest(t, callbackDone)
}
