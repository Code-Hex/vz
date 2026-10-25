package vz_test

import (
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

type customVirtioGuestDelegate struct {
	customVirtioTestDelegate
	stopped chan struct{}
}

func (d *customVirtioGuestDelegate) WillStop(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {
	select {
	case d.stopped <- struct{}{}:
	default:
	}
}

func TestCustomVirtioGuestLifecycle(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	delegate := &customVirtioGuestDelegate{
		customVirtioTestDelegate: customVirtioTestDelegate{
			created: make(chan *vz.CustomVirtioDevice, 1),
		},
		stopped: make(chan struct{}, 1),
	}
	provider, err := vz.NewCustomVirtioDeviceDelegateProvider(delegate)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	custom, err := vz.NewCustomVirtioDeviceConfiguration(0xff, 0xff, 0, 1, vz.WithCustomVirtioProvider(provider))
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
		return vz.WithCustomVirtioDevices(custom)(config)
	})
	defer container.Shutdown()
	select {
	case <-delegate.created:
	case <-time.After(5 * time.Second):
		t.Fatal("custom device creation was not reported")
	}
	if err := container.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-delegate.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("custom device stop was not reported")
	}
}
