package vz

import (
	"net"
	"reflect"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
)

func TestRuntimeNetworkDevice(t *testing.T) {
	if macOSAvailable(12) != nil {
		t.Skip("requires macOS 12")
	}
	nat, err := NewNATNetworkDeviceAttachment()
	if err != nil {
		t.Fatal(err)
	}
	config := newTestConfig(t)
	network, err := NewVirtioNetworkDeviceConfiguration(nat)
	if err != nil {
		t.Fatal(err)
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*VirtioNetworkDeviceConfiguration{network})
	vm, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	devices := vm.NetworkDevices()
	if len(devices) != 1 {
		t.Fatalf("got %d network devices, want 1", len(devices))
	}
	device := devices[0]
	initial := device.Attachment()
	if _, ok := initial.(*NATNetworkDeviceAttachment); !ok {
		t.Fatalf("initial attachment = %T, want NAT", initial)
	}
	objc.Release(initial)
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	file, err := socket.File()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	attachment, err := NewFileHandleNetworkDeviceAttachment(file)
	if err != nil {
		t.Fatal(err)
	}
	wantMTU := 1500
	if macOSAvailable(13) == nil {
		wantMTU = 2000
		if err := attachment.SetMaximumTransmissionUnit(wantMTU); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []NetworkDeviceAttachment{attachment, nil, nat} {
		if err := device.SetAttachment(want); err != nil {
			t.Fatal(err)
		}
		got := device.Attachment()
		if reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Fatalf("attachment = %T, want %T", got, want)
		}
		if objc.Ptr(got) != objc.Ptr(want) {
			t.Fatalf("attachment did not change to %T", want)
		}
		if fileAttachment, ok := got.(*FileHandleNetworkDeviceAttachment); ok {
			if mtu := fileAttachment.MaximumTransmissionUnit(); mtu != wantMTU {
				t.Errorf("MTU = %d, want %d", mtu, wantMTU)
			}
		}
		if got != nil {
			objc.Release(got)
		}
	}
}
