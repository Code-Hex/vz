package vmnetsock_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/macos"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnetsock"
	"golang.org/x/sys/unix"
)

func duplicate(t *testing.T, adapter *vmnetsock.Adapter) *os.File {
	t.Helper()
	file, err := adapter.Dup()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func TestAdapterDuplicateOwnership(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	for _, transport := range []vmnetsock.Transport{vmnetsock.Stream, vmnetsock.Datagram} {
		t.Run(fmt.Sprint(transport), func(t *testing.T) {
			configuration, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
			if err != nil {
				t.Fatal(err)
			}
			network, err := vmnet.NewNetwork(configuration)
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := vmnetsock.New(context.Background(), newInterface(t, network), transport)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			first := duplicate(t, adapter)
			second := duplicate(t, adapter)
			if first == second || first.Fd() == second.Fd() {
				t.Fatal("Dup returned the same descriptor")
			}
			flags, err := unix.FcntlInt(second.Fd(), unix.F_GETFD, 0)
			if err != nil || flags&unix.FD_CLOEXEC == 0 {
				t.Fatalf("descriptor flags = %d, error = %v", flags, err)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := second.Stat(); err != nil {
				t.Fatalf("closing one duplicate closed another: %v", err)
			}
			adapterCopy := *adapter
			closed := make(chan error, 2)
			for _, closeAdapter := range []func() error{adapterCopy.Close, adapter.Close} {
				go func() { closed <- closeAdapter() }()
			}
			for range 2 {
				select {
				case err := <-closed:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("Close did not finish")
				}
			}
			if _, err := second.Stat(); err != nil {
				t.Fatalf("Adapter.Close closed caller-owned descriptor: %v", err)
			}
			for _, a := range []*vmnetsock.Adapter{adapter, &adapterCopy} {
				if file, err := a.Dup(); file != nil || !errors.Is(err, os.ErrClosed) {
					t.Fatalf("Dup after Close = %v, %v", file, err)
				}
			}
		})
	}
}

func TestAdapterConcurrentDupAndClose(t *testing.T) {
	if macos.Available(26, vzbridge.SDKVersion) != nil {
		t.Skip("requires macOS 26 SDK")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, transport := range []vmnetsock.Transport{vmnetsock.Stream, vmnetsock.Datagram} {
		adapter, err := vmnetsock.New(context.Background(), newInterface(t, network), transport)
		if err != nil {
			t.Fatal(err)
		}
		defer adapter.Close()
		copy := *adapter
		start := make(chan struct{})
		ready := make(chan struct{}, 16)
		var workers sync.WaitGroup
		for range 16 {
			workers.Go(func() {
				first, err := copy.Dup()
				ready <- struct{}{}
				if err != nil {
					t.Error(err)
					return
				}
				defer first.Close()
				<-start
				for range 100 {
					file, err := copy.Dup()
					if errors.Is(err, os.ErrClosed) {
						return
					}
					if err != nil {
						t.Error(err)
						return
					}
					if _, err := file.Stat(); err != nil {
						t.Errorf("duplicate invalidated by concurrent Close: %v", err)
					}
					file.Close()
				}
			})
		}
		for range 16 {
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				close(start)
				t.Fatal("initial Dup did not finish")
			}
		}
		close(start)
		closed := make(chan error, 1)
		go func() { closed <- adapter.Close() }()
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Close did not finish")
		}
		done := make(chan struct{})
		go func() {
			workers.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("Dup did not finish")
		}
	}
}

func TestAdapterZeroValue(t *testing.T) {
	for _, adapter := range []*vmnetsock.Adapter{nil, {}} {
		if file, err := adapter.Dup(); file != nil || !errors.Is(err, os.ErrClosed) {
			t.Fatalf("uninitialized Dup = %v, %v", file, err)
		}
		if err := adapter.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
