package vz

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

func TestDiskImageServerClose(t *testing.T) {
	image, err := diskimagekit.CreateRAW(filepath.Join(t.TempDir(), "disk.raw"), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	server, err := NewDiskImageServer(image)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	serverURL, err := url.Parse(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(serverURL.Query().Get("socket"))
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if err := server.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket directory remains: %v", err)
	}
	if _, err := image.WriteAt([]byte("still owned"), 0); err != nil {
		t.Fatalf("server closed borrowed image: %v", err)
	}
}

func TestDiskImageServerListener(t *testing.T) {
	for _, network := range []string{"unix", "tcp"} {
		t.Run(network, func(t *testing.T) {
			image, err := diskimagekit.CreateRAW(filepath.Join(t.TempDir(), "disk.raw"), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer image.Close()
			var opts []DiskImageServerOption
			var listener net.Listener
			if network == "tcp" {
				listener, err = net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				serverURL := "nbd://" + listener.Addr().String() + "/"
				opts = append(opts, WithDiskImageServerListener(listener, serverURL))
			}
			server, err := NewDiskImageServer(image, opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			serverURL, err := url.Parse(server.URL())
			if err != nil {
				t.Fatal(err)
			}
			address := serverURL.Host
			if network == "unix" {
				if serverURL.Scheme != "nbd+unix" {
					t.Fatalf("URL scheme = %q, want nbd+unix", serverURL.Scheme)
				}
				address = serverURL.Query().Get("socket")
			} else if server.URL() != "nbd://"+listener.Addr().String()+"/" {
				t.Fatalf("unexpected custom URL: %s", server.URL())
			}
			conn, err := net.DialTimeout(network, address, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			var greeting [18]byte
			if _, err := io.ReadFull(conn, greeting[:]); err != nil {
				t.Fatal(err)
			}
			if string(greeting[:8]) != "NBDMAGIC" || binary.BigEndian.Uint64(greeting[8:16]) != 0x49484156454f5054 {
				t.Fatalf("invalid NBD greeting: %x", greeting)
			}
			if listener != nil {
				if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if err := server.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Read(greeting[:]); !errors.Is(err, io.EOF) {
				t.Errorf("connection after Close: %v", err)
			}
			if listener != nil {
				if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
					t.Errorf("listener after Close: %v", err)
				}
			}
		})
	}
}

func TestDiskImageServerListenerOwnership(t *testing.T) {
	image, err := diskimagekit.CreateRAW(filepath.Join(t.TempDir(), "disk.raw"), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	option := WithDiskImageServerListener(listener, "nbd://"+listener.Addr().String()+"/")
	server, err := NewDiskImageServer(nil, option)
	if err == nil {
		server.Close()
		t.Fatal("accepted nil image")
	}
	assertDiskImageListenerOpen(t, listener.(*net.TCPListener))
	selected, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Close()
	server, err = NewDiskImageServer(image, option,
		WithDiskImageServerListener(selected, "nbd://"+selected.Addr().String()+"/"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiskImageListenerOpen(t, listener.(*net.TCPListener))
}

func assertDiskImageListenerOpen(t *testing.T, listener *net.TCPListener) {
	t.Helper()
	if err := listener.SetDeadline(time.Now().Add(time.Millisecond)); err != nil {
		t.Fatal("caller listener was closed:", err)
	}
	if conn, err := listener.Accept(); err == nil {
		conn.Close()
		t.Fatal("unexpected client")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatal("caller listener is unusable:", err)
	}
	if err := listener.SetDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestDiskImageServerInvalidInputs(t *testing.T) {
	image, err := diskimagekit.CreateRAW(filepath.Join(t.TempDir(), "disk.raw"), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for _, test := range []struct {
		name   string
		option DiskImageServerOption
	}{
		{"nil option", nil},
		{"nil listener", WithDiskImageServerListener(nil, "nbd://127.0.0.1/")},
		{"empty URL", WithDiskImageServerListener(listener, "")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if server, err := NewDiskImageServer(image, test.option); err == nil {
				server.Close()
				t.Fatal("accepted invalid option")
			}
		})
	}
	if err := image.Close(); err != nil {
		t.Fatal(err)
	}
	if server, err := NewDiskImageServer(image); !errors.Is(err, os.ErrClosed) {
		if server != nil {
			server.Close()
		}
		t.Fatalf("closed image error = %v", err)
	}
}

func TestDiskImageServerAcceptError(t *testing.T) {
	image, err := diskimagekit.CreateRAW(filepath.Join(t.TempDir(), "disk.raw"), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	server, err := NewDiskImageServer(image,
		WithDiskImageServerListener(listener, "nbd://"+listener.Addr().String()+"/"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	select {
	case <-server.served:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit after Accept failed")
	}
	for range 2 {
		var netErr net.Error
		if err := server.Close(); !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("Close error = %v, want Accept timeout", err)
		}
	}
}
