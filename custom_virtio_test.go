package vz_test

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func TestCustomVirtioConfiguration(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	data := []byte{1, 2, 3, 4}
	specific, err := vz.NewVirtioDeviceSpecificConfiguration(data)
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 99
	if got := specific.ConfigurationData(); !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("configuration data = %v", got)
	}
	region, err := vz.NewVirtioSharedMemoryRegionConfiguration(2, 65536)
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewCustomVirtioDeviceConfiguration(4, 0xff, 0, 1,
		vz.WithCustomVirtioMandatoryFeatures(vz.VirtioFeatureSet{Subset0: 1}),
		vz.WithCustomVirtioOptionalFeatures(vz.VirtioFeatureSet{Subset0: 2}),
		vz.WithCustomVirtioDeviceSpecificConfiguration(specific),
		vz.WithCustomVirtioSharedMemoryRegions(region),
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.DeviceID() != 4 || config.PCIClassID() != 0xff || config.PCISubclassID() != 0 || config.VirtioQueueCount() != 1 {
		t.Fatal("device identifiers or queue count changed")
	}
	if config.MandatoryFeatures().Subset0 != 1 || config.OptionalFeatures().Subset0&2 != 2 {
		t.Fatalf("features mandatory=%#v optional=%#v", config.MandatoryFeatures(), config.OptionalFeatures())
	}
	if got := config.DeviceSpecificConfiguration().ConfigurationData(); !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("device data = %v", got)
	}
	regions := config.SharedMemoryRegions()
	if len(regions) != 1 || regions[0].RegionID() != 2 || regions[0].Size() != 65536 {
		t.Fatalf("shared memory regions = %v", regions)
	}
}

type customVirtioTestDelegate struct {
	created  chan *vz.CustomVirtioDevice
	contexts chan vz.CustomVirtioContext
	unblock  <-chan struct{}
	returned chan struct{}
}

func (d *customVirtioTestDelegate) DidCreateDevice(ctx vz.CustomVirtioContext, device *vz.CustomVirtioDevice) {
	ctx.RunOnDeviceQueue(func() error {
		device.NegotiatedFeatures()
		device.QueueAtIndex(0)
		device.SharedMemoryRegions()
		return nil
	})
	if d.contexts != nil {
		d.contexts <- ctx
	}
	d.created <- device
	if d.unblock != nil {
		<-d.unblock
	}
	if d.returned != nil {
		close(d.returned)
	}
}

func (*customVirtioTestDelegate) DidReceiveNotificationForQueue(vz.CustomVirtioContext, *vz.CustomVirtioDevice, *vz.VirtioQueue) {
}

func (*customVirtioTestDelegate) DidAcceptDriverOK(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {}

func (*customVirtioTestDelegate) WillStop(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {}

func (*customVirtioTestDelegate) WillPause(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {}

func (*customVirtioTestDelegate) WillResume(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {}

func (*customVirtioTestDelegate) WillReset(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {}

func TestCustomVirtioProvider(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	unblock := make(chan struct{})
	delegate := &customVirtioTestDelegate{
		created:  make(chan *vz.CustomVirtioDevice, 1),
		unblock:  unblock,
		returned: make(chan struct{}),
	}
	releaseCallback := sync.OnceFunc(func() { close(unblock) })
	provider, err := vz.NewCustomVirtioDeviceDelegateProvider(delegate)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	closeStarted := make(chan struct{})
	var closedDuringCallback bool
	startClose := sync.OnceFunc(func() {
		go func() {
			close(closeStarted)
			provider.Close()
			select {
			case <-delegate.returned:
			default:
				closedDuringCallback = true
			}
			provider.Close()
			close(closed)
		}()
	})
	t.Cleanup(func() {
		releaseCallback()
		startClose()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Error("provider cleanup did not finish")
		}
	})
	config := customVirtioMachineConfiguration(t, provider)
	machineReady := make(chan *vz.VirtualMachine, 1)
	machineError := make(chan error, 1)
	go func() {
		machine, err := vz.NewVirtualMachine(config)
		if err != nil {
			machineError <- err
			return
		}
		machineReady <- machine
	}()
	select {
	case <-delegate.created:
	case err := <-machineError:
		releaseCallback()
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		releaseCallback()
		t.Fatal("device creation callback did not return")
	}
	startClose()
	<-closeStarted
	releaseCallback()
	select {
	case <-closed:
		if closedDuringCallback {
			t.Fatal("Close returned while the delegate was running")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish after the callback returned")
	}

	var machine *vz.VirtualMachine
	select {
	case machine = <-machineReady:
	case err := <-machineError:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("virtual machine construction did not finish")
	}

	runtime.KeepAlive(machine)
}

func customVirtioMachineConfiguration(t *testing.T, provider *vz.CustomVirtioDeviceDelegateProvider) *vz.VirtualMachineConfiguration {
	t.Helper()
	specific, err := vz.NewVirtioDeviceSpecificConfiguration([]byte{0})
	if err != nil {
		t.Fatal(err)
	}
	custom, err := vz.NewCustomVirtioDeviceConfiguration(4, 0xff, 0, 1, vz.WithCustomVirtioProvider(provider), vz.WithCustomVirtioDeviceSpecificConfiguration(specific))
	if err != nil {
		t.Fatal(err)
	}
	store, err := vz.NewEFIVariableStore(t.TempDir()+"/efi", vz.WithCreatingEFIVariableStore())
	if err != nil {
		t.Fatal(err)
	}
	boot, err := vz.NewEFIBootLoader(vz.WithEFIVariableStore(store))
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtualMachineConfiguration(boot, 1, 1024*1024*1024, vz.WithCustomVirtioDevices(custom))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestCustomVirtioContext(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	type contextKey struct{}
	parent, cancel := context.WithTimeout(context.WithValue(context.Background(), contextKey{}, "parent"), time.Minute)
	defer cancel()
	for _, withParent := range []bool{false, true} {
		name := "background"
		var options []vz.CustomVirtioDeviceDelegateProviderOption
		if withParent {
			name = "parent"
			options = append(options, vz.WithCustomVirtioContext(parent))
		}
		t.Run(name, func(t *testing.T) {
			delegate := &customVirtioTestDelegate{
				created:  make(chan *vz.CustomVirtioDevice, 1),
				contexts: make(chan vz.CustomVirtioContext, 1),
			}
			provider, err := vz.NewCustomVirtioDeviceDelegateProvider(delegate, options...)
			if err != nil {
				t.Fatal(err)
			}
			defer provider.Close()
			machine, err := vz.NewVirtualMachine(customVirtioMachineConfiguration(t, provider))
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.KeepAlive(machine)
			ctx := <-delegate.contexts
			device := <-delegate.created
			if withParent {
				deadline, ok := ctx.Deadline()
				want, _ := parent.Deadline()
				if !ok || !deadline.Equal(want) || ctx.Done() != parent.Done() || ctx.Value(contextKey{}) != "parent" {
					t.Fatal("delegate context does not forward its parent")
				}
				cancel()
				if ctx.Err() != context.Canceled {
					t.Fatalf("Err = %v, want context.Canceled", ctx.Err())
				}
			} else if _, ok := ctx.Deadline(); ok || ctx.Done() != nil || ctx.Err() != nil {
				t.Fatal("default context is not a background context")
			}
			if !withParent {
				return
			}
			wantErr := errors.New("queue operation")
			if err := ctx.RunOnDeviceQueue(func() error {
				device.NegotiatedFeatures()
				return ctx.RunOnDeviceQueue(func() error { return wantErr })
			}); err != wantErr {
				t.Fatalf("nested queue operation = %v, want original error", err)
			}
			func() {
				defer func() {
					if got := recover(); got != wantErr {
						t.Errorf("recovered panic = %v, want original error", got)
					}
				}()
				ctx.RunOnDeviceQueue(func() error { panic(wantErr) })
			}()
			updated, err := vz.NewVirtioDeviceSpecificConfiguration([]byte{1})
			if err != nil {
				t.Fatal(err)
			}
			completed := make(chan error, 1)
			if err := ctx.RunOnDeviceQueue(func() error {
				return device.UpdateDeviceSpecificConfiguration(updated, func(err error) {
					completed <- err
				})
			}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-completed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("configuration update did not complete after RunOnDeviceQueue returned")
			}
			var count int
			var workers sync.WaitGroup
			for i := 0; i < 4; i++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for j := 0; j < 20; j++ {
						ctx.RunOnDeviceQueue(func() error {
							count++
							return nil
						})
					}
				}()
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				provider.Close()
			}()
			workers.Wait()
			if count != 80 {
				t.Fatalf("serialized updates = %d, want 80", count)
			}
		})
	}
	if provider, err := vz.NewCustomVirtioDeviceDelegateProvider(&customVirtioTestDelegate{}, vz.WithCustomVirtioContext(nil)); err == nil {
		provider.Close()
		t.Fatal("nil parent context was accepted")
	}
}
