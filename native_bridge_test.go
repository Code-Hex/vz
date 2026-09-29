//go:build darwin

package vz

import (
	"bytes"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
)

func TestNativeBridgeStringOwnership(t *testing.T) {
	for _, test := range []struct{ name, value string }{
		{"unicode", strings.Repeat("\u65e5\u672c\u8a9e\U0001f600", 500)},
		{"embedded NUL", "before\x00after"},
		{"empty", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			characters := utf16.Encode([]rune(test.value))
			length := len(characters)
			if len(characters) == 0 {
				characters = []uint16{0}
			}
			class := pureobjc.ID(pureobjc.GetClass("NSString"))
			allocated := class.Send(pureobjc.RegisterName("alloc"))
			address := pureobjc.Send[unsafe.Pointer](allocated, pureobjc.RegisterName("initWithCharacters:length:"), unsafe.Pointer(&characters[0]), uintptr(length))
			runtime.KeepAlive(characters)
			object := objc.NewManagedPointer(address, vzbridge.ReleaseObject)
			if got := nativeString(object); got != test.value {
				t.Fatalf("native string = %q, want %q", got, test.value)
			}
			vzbridge.Drain()
		})
	}
}

func TestNativeBridgeFacadeCopySurvivesCollection(t *testing.T) {
	alias, originalCollected := copiedNativeMACAddress(t)
	awaitNativeBridge(t, func() bool {
		select {
		case <-originalCollected:
			return true
		default:
			return false
		}
	})
	if got := alias.String(); got != "02:00:00:00:00:41" {
		t.Fatalf("copied MAC address = %q", got)
	}
	if got := alias.HardwareAddr(); !bytes.Equal(got, net.HardwareAddr{2, 0, 0, 0, 0, 0x41}) {
		t.Fatalf("copied hardware address = %v", got)
	}
	runtime.KeepAlive(alias)
}

func copiedNativeMACAddress(t *testing.T) (MACAddress, <-chan struct{}) {
	t.Helper()
	original, err := NewMACAddress(net.HardwareAddr{2, 0, 0, 0, 0, 0x41})
	if err != nil {
		t.Fatal(err)
	}
	collected := make(chan struct{})
	runtime.AddCleanup(original, func(ch chan struct{}) { close(ch) }, collected)
	return *original, collected
}

func TestNativeBridgeVMCollectionClosesSubscriptions(t *testing.T) {
	runtime.GC()
	vzbridge.Drain()
	before := nativeCallbacks.next.Load()
	states := abandonedNativeVM(t)
	awaitNativeBridge(t, func() bool {
		select {
		case _, open := <-states:
			return !open && nativeBridgeCallbackCountAfter(before) == 0
		default:
			return false
		}
	})
}

func TestNativeBridgeStateEvents(t *testing.T) {
	loader, err := NewLinuxBootLoader("testdata/Image", WithInitrd("testdata/initramfs.cpio.gz"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := NewVirtualMachineConfiguration(loader, 1, 512*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := machine.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if machine.CanStop() {
			if err := machine.Stop(); err != nil {
				t.Error(err)
			}
		}
	})
	deadline := time.After(5 * time.Second)
	for {
		select {
		case state := <-machine.StateChangedNotify():
			if state == VirtualMachineStateRunning {
				if machine.State() != VirtualMachineStateRunning {
					t.Fatal("state notification and current state disagree")
				}
				return
			}
		case <-deadline:
			t.Fatal("running VM did not emit its running state")
		}
	}
}

func abandonedNativeVM(t *testing.T) <-chan VirtualMachineState {
	t.Helper()
	machine, err := NewVirtualMachine(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if !machine.CanStart() {
		t.Fatal("new virtual machine cannot start")
	}
	return machine.StateChangedNotify()
}

func TestNativeBridgeNetworkAttachmentFailureReleasesRequest(t *testing.T) {
	if macOSAvailable(14) != nil {
		t.Skip("network block attachments require macOS 14")
	}
	runtime.GC()
	vzbridge.Drain()
	before := nativeCallbacks.next.Load()
	attachment, err := NewNetworkBlockDeviceStorageDeviceAttachment("", time.Second, true, DiskSynchronizationModeFull)
	if attachment != nil || err == nil {
		t.Fatalf("invalid URL returned attachment %v and error %v", attachment, err)
	}
	var nativeError *NSError
	if !errors.As(err, &nativeError) || nativeError.Domain == "" || nativeError.LocalizedDescription == "" {
		t.Fatalf("native error fields were lost: %#v", err)
	}
	if got := nativeBridgeCallbackCountAfter(before); got != 0 {
		t.Fatalf("callback count after failed attachment = %d, want 0", got)
	}
}

func TestNativeBridgeNetworkAttachmentCollectionClosesSubscriptions(t *testing.T) {
	if macOSAvailable(14) != nil {
		t.Skip("network block attachments require macOS 14")
	}
	runtime.GC()
	vzbridge.Drain()
	before := nativeCallbacks.next.Load()
	connected, failures := abandonedNativeNetworkAttachment(t)
	connectedClosed, failuresClosed := false, false
	awaitNativeBridge(t, func() bool {
		if !connectedClosed {
			select {
			case _, open := <-connected:
				connectedClosed = !open
			default:
			}
		}
		if !failuresClosed {
			select {
			case _, open := <-failures:
				failuresClosed = !open
			default:
			}
		}
		return connectedClosed && failuresClosed && nativeBridgeCallbackCountAfter(before) == 0
	})
}

func abandonedNativeNetworkAttachment(t *testing.T) (<-chan struct{}, <-chan error) {
	t.Helper()
	attachment, err := NewNetworkBlockDeviceStorageDeviceAttachment("nbd://127.0.0.1:10809/export", time.Second, true, DiskSynchronizationModeFull)
	if err != nil {
		t.Fatal(err)
	}
	return attachment.Connected(), attachment.DidEncounterError()
}

func nativeBridgeCallbackCountAfter(mark uint64) int {
	nativeCallbacks.mu.Lock()
	defer nativeCallbacks.mu.Unlock()
	count := 0
	for id := range nativeCallbacks.entries {
		if id > mark {
			count++
		}
	}
	return count
}

func awaitNativeBridge(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		vzbridge.Drain()
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("native cleanup did not complete within 10 seconds")
}
