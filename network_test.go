package vz_test

import (
	"net"
	"testing"

	"github.com/Code-Hex/vz/v2"
)

func TestFileHandleNetworkDeviceAttachmentMTU(t *testing.T) {
	if vz.MacosMajorVersionLessThan(13) {
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
