//go:build darwin

package vz

import (
	"errors"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestNetworkDisconnectedNotifications(t *testing.T) {
	if macOSAvailable(12) != nil {
		t.Skip("requires macOS 12")
	}
	attachment, err := NewNATNetworkDeviceAttachment()
	if err != nil {
		t.Fatal(err)
	}
	network, err := NewVirtioNetworkDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	config := newTestConfig(t)
	config.SetNetworkDevicesVirtualMachineConfiguration([]*VirtioNetworkDeviceConfiguration{network})
	machine, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(machine.pointer)
	domain := vzbridge.NSString_StringWithUTF8String("VZErrorDomain")
	defer objc.Release(domain)
	failure := vzbridge.NSError_ErrorWithDomain_Code_UserInfo(domain, 7, nil)
	defer objc.Release(failure)
	emit := func() {
		onMachineQueue(t, machine, func() {
			receiver := pureobjc.ID(uintptr(objc.Ptr(machine)))
			delegate := receiver.Send(pureobjc.RegisterName("delegate"))
			devices := receiver.Send(pureobjc.RegisterName("networkDevices"))
			device := devices.Send(pureobjc.RegisterName("objectAtIndex:"), uintptr(0))
			for range 20 {
				delegate.Send(pureobjc.RegisterName("virtualMachine:networkDevice:attachmentWasDisconnectedWithError:"), receiver, device, objc.Ptr(failure))
				delegate.Send(pureobjc.RegisterName("virtualMachine:networkDevice:attachmentWasDisconnectedWithError:"), receiver, pureobjc.ID(0), objc.Ptr(failure))
			}
		})
	}
	notifications := make(chan *DisconnectedError, 40)
	stop, err := machine.NetworkDeviceAttachmentWasDisconnected(notifications)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	emit()
	for i := range 40 {
		select {
		case event := <-notifications:
			var failure *NSError
			if event == nil || !errors.As(event, &failure) || failure.Code != 7 {
				t.Fatalf("notification %d = %v", i, event)
			}
			want := network
			if i%2 != 0 {
				want = nil
			}
			if event.Config != want {
				t.Fatalf("notification %d configuration = %p, want %p", i, event.Config, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("notification %d was not delivered", i)
		}
	}
	emit()
	objc.Release(machine.pointer)
	vzbridge.Drain()
	stop()
	if event := <-notifications; event == nil {
		t.Fatal("release discarded caller buffer")
	}
	close(notifications)
}
