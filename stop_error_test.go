//go:build darwin

package vz

import (
	"errors"
	"runtime"
	"runtime/cgo"
	"testing"
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
	defer objc.Release(machine.pointer)
	stops := make(chan struct{}, 1)
	stopGuestStopped, err := machine.GuestStoppedNotify(stops)
	if err != nil {
		t.Fatal(err)
	}
	defer stopGuestStopped()
	failures := make(chan error, 1)
	stop, err := machine.StoppedWithErrorNotify(failures)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	domain := vzbridge.NSString_StringWithUTF8String("VZErrorDomain")
	defer objc.Release(domain)
	nativeError := vzbridge.NSError_ErrorWithDomain_Code_UserInfo(domain, 7, nil)
	defer objc.Release(nativeError)
	onMachineQueue(t, machine, func() {
		receiver := pureobjc.ID(uintptr(objc.Ptr(machine)))
		receiver.Send(pureobjc.RegisterName("guestDidStopVirtualMachine:"), receiver)
	})
	select {
	case <-stops:
	default:
		t.Fatal("missing guest shutdown notification")
	}
	select {
	case err := <-failures:
		t.Fatalf("guest shutdown reported an error: %v", err)
	default:
	}
	onMachineQueue(t, machine, func() {
		receiver := pureobjc.ID(uintptr(objc.Ptr(machine)))
		receiver.Send(pureobjc.RegisterName("virtualMachine:didStopWithError:"), receiver, objc.Ptr(nativeError))
	})
	select {
	case err := <-failures:
		var native *NSError
		if !errors.As(err, &native) || native.Domain != "VZErrorDomain" || native.Code != 7 {
			t.Fatalf("stop error = %v", err)
		}
	default:
		t.Fatal("missing stop error")
	}
	select {
	case <-stops:
		t.Fatal("fatal error reported a normal guest shutdown")
	default:
	}
	objc.Release(machine.pointer)
	vzbridge.Drain()
	stop()
	failure := errors.New("caller value")
	failures <- failure
	if got := <-failures; got != failure {
		t.Fatalf("caller channel changed after release: %v", got)
	}
	close(failures)
}

func TestVirtualMachineReleaseWithUnreadNotifications(t *testing.T) {
	machine, err := NewVirtualMachine(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	failures := make(chan error, 1)
	stop, err := machine.StoppedWithErrorNotify(failures)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("unread stop error")
	for range 100 {
		machine.machineState.stopErrors.send(failure)
	}
	objc.Release(machine.pointer)
	vzbridge.Drain()
	stop()
	if got := <-failures; got != failure {
		t.Fatalf("caller buffer was not retained: %v", got)
	}
	close(failures)
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
