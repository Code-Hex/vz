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
	for range 100 {
		emit()
	}
	for i := range 100 {
		select {
		case _, open := <-attachment.Connected():
			if !open {
				t.Fatalf("connection channel closed at notification %d", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("connection notification %d was not delivered", i)
		}
		select {
		case err := <-attachment.DidEncounterError():
			var native *NSError
			if !errors.As(err, &native) || native.Domain != "NSPOSIXErrorDomain" || native.Code != 5 {
				t.Fatalf("error notification %d = %#v", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("error notification %d was not delivered", i)
		}
	}
}

func TestNetworkBlockDeviceReleaseWithUnreadNotifications(t *testing.T) {
	attachment, emit := newNetworkBlockDeviceNotificationTest(t)
	for range 100 {
		emit()
	}
	objc.Release(attachment.pointer)
	vzbridge.Drain()
	select {
	case _, open := <-attachment.Connected():
		if open {
			t.Fatal("released attachment retained an unread connection notification")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connection channel remained open after attachment release")
	}
	select {
	case err, open := <-attachment.DidEncounterError():
		if open {
			t.Fatalf("released attachment retained an unread error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("error channel remained open after attachment release")
	}
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
