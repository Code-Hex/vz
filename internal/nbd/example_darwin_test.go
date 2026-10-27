package nbd_test

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/diskimagekit"
	"github.com/Code-Hex/vz/v4/internal/nbd"
)

func ExampleNewServer() {
	image, err := diskimagekit.Open("base.asif", diskimagekit.WithOpenMode(diskimagekit.ReadOnly))
	if err != nil {
		panic(err)
	}
	defer image.Close()
	if err := image.CreateLayer("overlay.asif", diskimagekit.Overlay); err != nil {
		panic(err)
	}

	dir, err := os.MkdirTemp("", "diskimage-nbd-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "disk.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	server, err := nbd.NewServer(image)
	if err != nil {
		panic(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	defer func() {
		server.Close()
		if err := <-served; !errors.Is(err, net.ErrClosed) {
			panic(err)
		}
	}()

	endpoint := url.URL{
		Scheme:   "nbd+unix",
		Path:     "/",
		RawQuery: url.Values{"socket": {socket}}.Encode(),
	}
	attachment, err := vz.NewNetworkBlockDeviceStorageDeviceAttachment(
		endpoint.String(), 5*time.Second, false, vz.DiskSynchronizationModeFull,
	)
	if err != nil {
		panic(err)
	}
	device, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
	if err != nil {
		panic(err)
	}
	_ = device
	// Add device to the VM configuration and keep this export open until
	// the VM stops. Close the server before closing the borrowed image.
}
