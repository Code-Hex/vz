//go:build darwin

package vz

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/objc"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestUSBPassthroughInvalidArguments(t *testing.T) {
	if macOSAvailable(27) != nil {
		t.Skip("requires macOS 27")
	}
	if config, err := NewXHCIControllerConfiguration(WithUSBDevices(nil)); config != nil || err == nil {
		t.Fatalf("nil startup device returned (%v, %v)", config, err)
	}
	if config, err := NewUSBPassthroughDeviceConfiguration(nil); config != nil || err == nil {
		t.Fatalf("nil accessory returned (%v, %v)", config, err)
	}
	if device, err := NewUSBPassthroughDevice(nil); device != nil || err == nil {
		t.Fatalf("nil configuration returned (%v, %v)", device, err)
	}
	if devices, stop, err := USBAccessoryChangedNotify(nil); devices != nil || stop != nil || err == nil {
		t.Fatalf("nil channel returned devices=%v, stop=%v, error=%v", devices, stop != nil, err)
	}
}

func TestUSBDisconnectRegistration(t *testing.T) {
	if macOSAvailable(27) != nil {
		t.Skip("requires macOS 27")
	}
	config := newTestConfig(t)
	usb, err := NewXHCIControllerConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	config.SetUSBControllersVirtualMachineConfiguration([]USBControllerConfiguration{usb})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	controller := vm.USBControllers()[0]
	path := t.TempDir() + "/usb.img"
	if err := os.WriteFile(path, make([]byte, 1024*1024), 0600); err != nil {
		t.Fatal(err)
	}
	attachment, err := NewDiskImageStorageDeviceAttachment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := NewUSBMassStorageDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	device, err := NewUSBMassStorageDevice(storage)
	if err != nil {
		t.Fatal(err)
	}
	emit := func() {
		onMachineQueue(t, vm, func() {
			receiver := pureobjc.ID(uintptr(objc.Ptr(controller)))
			delegate := receiver.Send(pureobjc.RegisterName("delegate"))
			delegate.Send(pureobjc.RegisterName("usbController:usbPassthroughDeviceDidDisconnect:"), receiver, objc.Ptr(device))
		})
	}
	channel := make(chan USBDevice, 1)
	stop, err := controller.USBPassthroughDeviceDidDisconnectNotify(channel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() {
			stop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("notification cleanup did not finish")
		}
	})
	if duplicate, err := controller.USBPassthroughDeviceDidDisconnectNotify(channel); duplicate != nil || err == nil {
		t.Fatalf("duplicate registration returned stop=%v, error=%v", duplicate != nil, err)
	}
	emit()
	select {
	case got := <-channel:
		if got.UUID() != device.UUID() {
			t.Fatalf("disconnected device = %s, want %s", got.UUID(), device.UUID())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect notification was not delivered")
	}
	sent := make(chan struct{})
	sending := make(chan struct{})
	go func() {
		emit()
		close(sending)
		for range 32 {
			emit()
		}
		close(sent)
	}()
	select {
	case <-sending:
	case <-time.After(5 * time.Second):
		t.Fatal("notification sender did not start")
	}
	var stoppers sync.WaitGroup
	for range 8 {
		stoppers.Go(stop)
	}
	stopped := make(chan struct{})
	go func() {
		stoppers.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("notification and stop did not finish")
	}
	close(channel)
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("notification sender did not finish")
	}
	emit()
	replacement := make(chan USBDevice, 1)
	stopReplacement, err := controller.USBPassthroughDeviceDidDisconnectNotify(replacement)
	if err != nil {
		t.Fatal(err)
	}
	stop = stopReplacement
	emit()
	select {
	case got := <-replacement:
		if got.UUID() != device.UUID() {
			t.Fatal("replacement received a different device")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not receive a notification")
	}
	replacementStopped := make(chan struct{})
	go func() {
		stopReplacement()
		close(replacementStopped)
	}()
	select {
	case <-replacementStopped:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement stop did not finish")
	}
	close(replacement)
}
