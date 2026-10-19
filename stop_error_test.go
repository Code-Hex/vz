//go:build darwin

package vz

import (
	"errors"
	"runtime"
	"runtime/cgo"
	"testing"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/ebitengine/purego"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestVirtualMachineStoppedWithErrorNotify(t *testing.T) {
	machine, err := NewVirtualMachine(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	failures := machine.StoppedWithErrorNotify()
	defer objc.Release(machine.pointer)
	domain := vzbridge.NSString_StringWithUTF8String("VZErrorDomain")
	defer objc.Release(domain)
	message := vzbridge.NSString_StringWithUTF8String("Virtual machine stopped unexpectedly")
	defer objc.Release(message)
	key := vzbridge.NSString_StringWithUTF8String("NSLocalizedDescription")
	defer objc.Release(key)
	info := vzbridge.NSDictionary_DictionaryWithObject_ForKey(message, key)
	defer objc.Release(info)
	nativeError := vzbridge.NSError_ErrorWithDomain_Code_UserInfo(domain, 7, info)
	defer objc.Release(nativeError)
	droppedError := vzbridge.NSError_ErrorWithDomain_Code_UserInfo(domain, 8, info)
	defer objc.Release(droppedError)

	emitStop := func(failure *objc.Pointer) {
		receiver := pureobjc.ID(uintptr(objc.Ptr(machine)))
		delegate := receiver.Send(pureobjc.RegisterName("delegate"))
		delegate.Send(pureobjc.RegisterName("virtualMachine:didStopWithError:"), receiver, objc.Ptr(failure))
	}
	onMachineQueue(t, machine, func() {
		receiver := pureobjc.ID(uintptr(objc.Ptr(machine)))
		delegate := receiver.Send(pureobjc.RegisterName("delegate"))
		delegate.Send(pureobjc.RegisterName("guestDidStopVirtualMachine:"), receiver)
	})
	select {
	case err := <-failures:
		t.Fatalf("normal guest shutdown reported an error: %v", err)
	default:
	}
	for i := range 2 {
		onMachineQueue(t, machine, func() {
			emitStop(nativeError)
			emitStop(droppedError)
		})
		if i == 1 {
			objc.Release(machine.pointer)
			vzbridge.Drain()
		}
		select {
		case err, open := <-failures:
			var native *NSError
			if !open || !errors.As(err, &native) || native.Domain != "VZErrorDomain" || native.Code != 7 || native.LocalizedDescription != "Virtual machine stopped unexpectedly" {
				t.Fatalf("stop error = %#v, open = %v", err, open)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("native stop error was not delivered")
		}
	}

	select {
	case err, open := <-failures:
		if open {
			t.Fatalf("unexpected additional stop error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop error channel remained open after machine release")
	}
}

func onMachineQueue(t *testing.T, machine *VirtualMachine, body func()) {
	t.Helper()
	library, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(library)
	var dispatchSync func(unsafe.Pointer, uintptr, uintptr)
	purego.RegisterLibFunc(&dispatchSync, library, "dispatch_sync_f")

	handle := cgo.NewHandle(body)
	defer handle.Delete()
	dispatchSync(vzbridge.ObjectQueue(machine), uintptr(handle), machineQueueTestCallback)
	runtime.KeepAlive(machine)
}

var machineQueueTestCallback = purego.NewCallback(func(context uintptr) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := pureobjc.ID(pureobjc.GetClass("NSAutoreleasePool")).Send(pureobjc.RegisterName("new"))
	defer pool.Send(pureobjc.RegisterName("release"))
	cgo.Handle(context).Value().(func())()
})
