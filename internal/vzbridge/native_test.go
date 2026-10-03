//go:build darwin

package vzbridge

import (
	"os"
	"sync"
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/ebitengine/purego"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestNativeQueueReentry(t *testing.T) {
	var result int
	onQueue(func() {
		result++
		onQueue(func() { result += 10 })
		result += 100
	})
	if result != 111 {
		t.Fatalf("queue reentry result = %d, want 111", result)
	}
}

func TestNativeQueuePanic(t *testing.T) {
	func() {
		defer func() {
			if got := recover(); got != "queue panic" {
				t.Errorf("panic = %v, want queue panic", got)
			}
		}()
		onQueue(func() { onQueue(func() { panic("queue panic") }) })
	}()
	var resumed bool
	onQueue(func() { resumed = true })
	if !resumed {
		t.Fatal("queue did not resume after panic")
	}
}

func TestNativeQueueSerializes(t *testing.T) {
	var count int
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				onQueue(func() { count++ })
			}
		}()
	}
	wg.Wait()
	if count != 240 {
		t.Fatalf("queue count = %d, want 240", count)
	}
}

func TestNativeFileDescriptorFailureClosesPartialDuplicate(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "native-fd")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	before := nativeOpenDescriptors(t)
	for i := 0; i < 30; i++ {
		var pointer unsafe.Pointer
		attachment := NewVZFileHandleSerialPortAttachment(int32(file.Fd()), -1, &pointer)
		if attachment != nil {
			objc.Release(attachment)
			t.Fatal("invalid write descriptor was accepted")
		}
		if pointer == nil {
			t.Fatal("invalid descriptor returned no error")
		}
		failure := objc.NewManagedPointer(pointer, ReleaseObject)
		if code := NSError_Code(failure); code != 9 {
			t.Fatalf("descriptor error = %d, want EBADF", code)
		}
		objc.Release(failure)
	}
	Drain()
	after := nativeOpenDescriptors(t)
	if len(after) != len(before) {
		t.Fatalf("open descriptors = %d, before failure = %d", len(after), len(before))
	}
	if _, err := file.WriteString("original remains open"); err != nil {
		t.Fatal(err)
	}
}

func nativeOpenDescriptors(t *testing.T) []string {
	t.Helper()
	Drain()
	directory, err := os.Open("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	names, err := directory.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func TestNativeListenerInvalidationAndRelease(t *testing.T) {
	var events []uint64
	var unexpected uint32
	callback := purego.NewCallback(func(kind uint32, context uint64, first, second unsafe.Pointer, value uint64) uint64 {
		if kind != 4 {
			unexpected = kind
		}
		events = append(events, context)
		onQueue(func() {})
		return 0
	})
	SetCallback(uint64(callback))
	defer SetCallback(0)
	listener := NewVZVirtioSocketListener(101)
	if listener == nil {
		t.Fatal("listener construction failed")
	}
	InvalidateVZVirtioSocketListener(listener)
	InvalidateVZVirtioSocketListener(listener)
	objc.Release(listener)
	Drain()
	listener = NewVZVirtioSocketListener(102)
	objc.Release(listener)
	Drain()
	if unexpected != 0 {
		t.Fatalf("unexpected native callback kind = %d", unexpected)
	}
	if len(events) != 2 || events[0] != 101 || events[1] != 102 {
		t.Fatalf("listener terminal events = %v, want [101 102]", events)
	}
}

func TestNativeDiskModeFailureReturnsOwnedError(t *testing.T) {
	var pointer unsafe.Pointer
	attachment := NewVZDiskBlockDeviceStorageDeviceAttachment(-1, true, -1, &pointer)
	if attachment != nil {
		objc.Release(attachment)
		t.Fatal("invalid disk mode was accepted")
	}
	if pointer == nil {
		t.Fatal("invalid disk mode returned no error")
	}
	failure := objc.NewManagedPointer(pointer, ReleaseObject)
	defer objc.Release(failure)
	if code := NSError_Code(failure); code != 22 && code != 45 {
		t.Fatalf("disk mode error = %d, want EINVAL or ENOTSUP", code)
	}
}

func TestNativeMachinePreservesNetworkDelegate(t *testing.T) {
	var disconnected int
	callback := purego.NewCallback(func(kind uint32, context uint64, first, second unsafe.Pointer, value uint64) uint64 {
		if kind == 3 && context == 702 && value == 0 {
			disconnected++
		}
		if first != nil {
			ReleaseObject(first)
		}
		return 0
	})
	SetCallback(uint64(callback))
	defer SetCallback(0)
	loader := VZEFIBootLoader_Init()
	var nativeError unsafe.Pointer
	store := VZEFIVariableStore_InitCreatingVariableStoreAtURL_Options_Error(NSURL_FileURLWithPath(NSString_StringWithUTF8String(t.TempDir()+"/efi-vars")), uint64(1), &nativeError)
	if store == nil {
		t.Fatal("EFI variable store creation failed")
	}
	VZEFIBootLoader_SetVariableStore(loader, store)
	config := NewVZVirtualMachineConfiguration(loader, 1, 512*1024*1024)
	attachment := VZNATNetworkDeviceAttachment_Init()
	network := VZVirtioNetworkDeviceConfiguration_Init()
	VZNetworkDeviceConfiguration_SetAttachment(network, attachment)
	devices := NSMutableArray_New()
	NSMutableArray_AddObject(devices, network)
	VZVirtualMachineConfiguration_SetNetworkDevices(config, devices)
	if !VZVirtualMachineConfiguration_ValidateWithError(config, &nativeError) {
		t.Fatal("delegate test configuration is invalid")
	}
	machine := NewVZVirtualMachineWithDispatchQueue(config, 0, 702)
	external := NSObject_New()
	defer func() {
		for _, object := range []*objc.Pointer{machine, external, devices, network, attachment, config, loader, store} {
			objc.Release(object)
		}
		Drain()
	}()
	var handlesDisconnection bool
	onQueue(func() {
		receiver := pureobjc.ID(uintptr(objc.Ptr(machine)))
		receiver.Send(pureobjc.RegisterName("setDelegate:"), objc.Ptr(external))
		delegate := receiver.Send(pureobjc.RegisterName("delegate"))
		selector := pureobjc.RegisterName("virtualMachine:networkDevice:attachmentWasDisconnectedWithError:")
		handlesDisconnection = pureobjc.Send[bool](delegate, pureobjc.RegisterName("respondsToSelector:"), selector)
		if handlesDisconnection {
			runtimeDevices := receiver.Send(pureobjc.RegisterName("networkDevices"))
			device := runtimeDevices.Send(pureobjc.RegisterName("objectAtIndex:"), uintptr(0))
			delegate.Send(selector, receiver, device, pureobjc.ID(0))
		}
	})
	if !handlesDisconnection {
		t.Fatal("replacing the UI delegate removed the network disconnection handler")
	}
	if disconnected != 1 {
		t.Fatalf("network disconnection notifications = %d, want 1", disconnected)
	}
}
