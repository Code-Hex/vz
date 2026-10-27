package vz_test

import (
	"bytes"
	"net"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestFileHandleNetworkDeviceAttachmentMTU(t *testing.T) {
	ln, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: 0,
		IP:   net.ParseIP("127.0.0.1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	f, err := ln.File()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	attachment, err := vz.NewFileHandleNetworkDeviceAttachment(f)
	if err != nil {
		t.Fatal(err)
	}
	got := attachment.MaximumTransmissionUnit()
	if got != 1500 {
		t.Fatalf("want default mtu 1500 but got %d", got)
	}

	if vz.Available(13) {
		return
	}
	want := 1600
	if err := attachment.SetMaximumTransmissionUnit(want); err != nil {
		t.Fatal(err)
	}

	got2 := attachment.MaximumTransmissionUnit()
	if got2 != want {
		t.Fatalf("want mtu %d but got %d", want, got2)
	}
}

func TestVirtioNetworkDeviceConfigurationGetMACAddress(t *testing.T) {
	attachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}

	want := net.HardwareAddr{0x02, 0x00, 0x5e, 0x10, 0x00, 0x01}
	macAddress, err := vz.NewMACAddress(want)
	if err != nil {
		t.Fatal(err)
	}
	config.SetMACAddress(macAddress)

	got := config.GetMACAddress()
	if got == nil {
		t.Fatal("want MAC address but got nil")
	}
	if got.String() != want.String() {
		t.Fatalf("want MAC address %q but got %q", want, got)
	}
	if gotHardwareAddr := got.HardwareAddr(); !bytes.Equal(gotHardwareAddr, want) {
		t.Fatalf("want hardware address %q but got %q", want, gotHardwareAddr)
	}
	replacement, err := vz.NewMACAddress(net.HardwareAddr{0x02, 0x00, 0x5e, 0x10, 0x00, 0x02})
	if err != nil {
		t.Fatal(err)
	}
	config.SetMACAddress(replacement)
	if got.String() != want.String() {
		t.Fatalf("want retained MAC address %q but got %q", want, got)
	}
}

func TestVirtioNetworkDeviceConfigurationGetDefaultMACAddress(t *testing.T) {
	attachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}

	got := config.GetMACAddress()
	if got == nil {
		t.Fatal("want default MAC address but got nil")
	}
	gotHardwareAddr, err := net.ParseMAC(got.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(gotHardwareAddr) != 6 {
		t.Fatalf("want 6-byte MAC address but got %d bytes", len(gotHardwareAddr))
	}
	if got.String() != gotHardwareAddr.String() {
		t.Fatalf("want colon-separated MAC address but got %q", got)
	}
}

func TestNewMACAddress(t *testing.T) {
	for _, address := range []net.HardwareAddr{
		{0x02, 0x00, 0x5e, 0x10, 0x00, 0x01},
		{0x01, 0x00, 0x5e, 0x00, 0x00, 0x01},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	} {
		t.Run(address.String(), func(t *testing.T) {
			got, err := vz.NewMACAddress(address)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("got nil MAC address")
			}
			if !bytes.Equal(got.HardwareAddr(), address) {
				t.Fatalf("MAC address = %s, want %s", got, address)
			}
		})
	}
}

func TestNewMACAddressInvalidLength(t *testing.T) {
	for _, address := range []net.HardwareAddr{
		nil,
		{},
		{0x02, 0x00},
		{0x02, 0x00, 0x5e, 0x10, 0x00},
		{0x02, 0x00, 0x5e, 0x10, 0x00, 0x01, 0x00},
		{0x02, 0x00, 0x5e, 0xff, 0xfe, 0x10, 0x00, 0x01},
	} {
		got, err := vz.NewMACAddress(address)
		if got != nil || err == nil {
			t.Errorf("NewMACAddress(%v) = (%v, %v), want (nil, error)", []byte(address), got, err)
		}
	}
}
