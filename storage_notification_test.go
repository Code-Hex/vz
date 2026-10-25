//go:build darwin

package vz

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestNetworkBlockDeviceNotifications(t *testing.T) {
	attachment, emit := newNetworkBlockDeviceNotificationTest(t)
	connected := make(chan struct{}, 2)
	failures := make(chan error, 2)
	stopConnected, err := attachment.WasConnectedNotify(connected)
	if err != nil {
		t.Fatal(err)
	}
	stopFailures, err := attachment.DidEncounterErrorNotify(failures)
	if err != nil {
		t.Fatal(err)
	}
	defer stopConnected()
	defer stopFailures()
	for range 100 {
		emit()
	}
	for range 2 {
		<-connected
		var native *NSError
		if err := <-failures; !errors.As(err, &native) || native.Domain != "NSPOSIXErrorDomain" || native.Code != 5 {
			t.Fatalf("error = %#v", err)
		}
	}
	emit()
	<-connected
	<-failures
	stopConnected()
	stopFailures()
	close(connected)
	close(failures)
	emit()
}

func TestNetworkBlockDeviceReleaseWithUnreadNotifications(t *testing.T) {
	attachment, emit := newNetworkBlockDeviceNotificationTest(t)
	connected := make(chan struct{}, 1)
	failures := make(chan error, 1)
	stopConnected, err := attachment.WasConnectedNotify(connected)
	if err != nil {
		t.Fatal(err)
	}
	stopFailures, err := attachment.DidEncounterErrorNotify(failures)
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		emit()
	}
	objc.Release(attachment.pointer)
	vzbridge.Drain()
	stopConnected()
	stopFailures()
	<-connected
	<-failures
	connected <- struct{}{}
	failures <- errors.New("caller owned")
	close(connected)
	close(failures)
}

func newNetworkBlockDeviceNotificationTest(t *testing.T) (*NetworkBlockDeviceStorageDeviceAttachment, func()) {
	t.Helper()
	if macOSAvailable(14) != nil {
		t.Skip("network block attachments require macOS 14")
	}
	attachment, err := NewNetworkBlockDeviceStorageDeviceAttachment("nbd://127.0.0.1:10809/export", time.Second, true, DiskSynchronizationModeFull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { objc.Release(attachment.pointer) })
	domain := vzbridge.NSString_StringWithUTF8String("NSPOSIXErrorDomain")
	t.Cleanup(func() { objc.Release(domain) })
	failure := vzbridge.NSError_ErrorWithDomain_Code_UserInfo(domain, 5, nil)
	t.Cleanup(func() { objc.Release(failure) })
	return attachment, func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pool := pureobjc.ID(pureobjc.GetClass("NSAutoreleasePool")).Send(pureobjc.RegisterName("new"))
		defer pool.Send(pureobjc.RegisterName("release"))
		receiver := pureobjc.ID(uintptr(objc.Ptr(attachment)))
		delegate := receiver.Send(pureobjc.RegisterName("delegate"))
		delegate.Send(pureobjc.RegisterName("attachmentWasConnected:"), receiver)
		delegate.Send(pureobjc.RegisterName("attachment:didEncounterError:"), receiver, objc.Ptr(failure))
	}
}
