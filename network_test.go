package vz_test

import (
	"bytes"
	"net"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestFileHandleNetworkDeviceAttachmentMTU(t *testing.T) {
	if vz.Available(13) {
		t.Skip("FileHandleNetworkDeviceAttachment.SetMaximumTransmissionUnit is supported from macOS 13")
	}

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

	attachment, err := vz.NewFileHandleNetworkDeviceAttachment(f)
	if err != nil {
		t.Fatal(err)
	}
	got := attachment.MaximumTransmissionUnit()
	if got != 1500 {
		t.Fatalf("want default mtu 1500 but got %d", got)
	}

	want := 2000
	if err := attachment.SetMaximumTransmissionUnit(want); err != nil {
		t.Fatal(err)
	}

	got2 := attachment.MaximumTransmissionUnit()
	if got2 != want {
		t.Fatalf("want mtu %d but got %d", want, got)
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

func TestMACAddressConstruction(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr net.HardwareAddr
		want string
	}{
		{name: "nil"},
		{name: "empty", addr: net.HardwareAddr{}},
		{name: "short", addr: net.HardwareAddr{2}},
		{name: "valid", addr: net.HardwareAddr{2, 0, 0, 0, 0, 1}, want: "02:00:00:00:00:01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mac, err := vz.NewMACAddress(tc.addr)
			if err != nil {
				t.Fatal(err)
			}
			if mac == nil {
				t.Fatal("hardware address returned a nil wrapper")
			}
			if got := mac.String(); got != tc.want {
				t.Fatalf("hardware address string = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMACAddressNilAndZero(t *testing.T) {
	var zero vz.MACAddress
	if got := zero.String(); got != "" {
		t.Fatalf("zero MAC address string = %q, want empty", got)
	}
	var nilMAC *vz.MACAddress
	assertGoPanic(t, func() { _ = nilMAC.String() })
	attachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	assertGoPanic(t, func() { config.SetMACAddress(nil) })
}
