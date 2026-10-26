package vz_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

type customVirtioIteratorDelegate struct {
	customVirtioTestDelegate
	breakEarly bool
	deviceID   uint64
	errors     chan error
}

func (d *customVirtioIteratorDelegate) DidCreateDevice(_ vz.CustomVirtioContext, device *vz.CustomVirtioDevice) {
	d.deviceID = device.ID()
}

func (d *customVirtioIteratorDelegate) DidReceiveNotificationForQueue(_ vz.CustomVirtioContext, device *vz.CustomVirtioDevice, queue *vz.VirtioQueue) {
	if device.ID() != d.deviceID {
		select {
		case d.errors <- fmt.Errorf("notification device ID = %d, want %d", device.ID(), d.deviceID):
		default:
		}
		return
	}
	elements := queue.Elements()
	if d.breakEarly {
		var first *vz.VirtioQueueElement
		for element := range elements {
			first = element
			break
		}
		if first != nil {
			d.fill(first)
		}
	}
	for element := range elements {
		d.fill(element)
	}
}

func (d *customVirtioIteratorDelegate) fill(element *vz.VirtioQueueElement) {
	copy := *element
	data := bytes.Repeat([]byte{0xa5}, int(min(element.WriteBuffersAvailableByteCount(), 64)))
	err := element.WriteBytes(data)
	if err == nil {
		err = element.ReturnToQueue()
	}
	if err == nil && copy.ReturnToQueue() == nil {
		err = fmt.Errorf("copied element accepted a second return")
	}
	if err == nil {
		if _, readErr := copy.ReadBytes(0); readErr == nil {
			err = fmt.Errorf("copied element accepted a read after return")
		}
	}
	if err == nil {
		if _, peekErr := copy.PeekBytes(0); peekErr == nil {
			err = fmt.Errorf("copied element accepted a peek after return")
		}
	}
	if err == nil && copy.WriteBytes(nil) == nil {
		err = fmt.Errorf("copied element accepted a write after return")
	}
	if err != nil {
		select {
		case d.errors <- err:
		default:
		}
	}
}

func TestVirtioQueueElementsGuest(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	for _, name := range []string{"drain", "break-and-resume"} {
		t.Run(name, func(t *testing.T) {
			delegate := &customVirtioIteratorDelegate{
				breakEarly: name == "break-and-resume",
				errors:     make(chan error, 1),
			}
			provider, err := vz.NewCustomVirtioDeviceDelegateProvider(delegate)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				provider.Close()
				select {
				case err := <-delegate.errors:
					t.Error(err)
				default:
				}
			}()
			custom, err := vz.NewCustomVirtioDeviceConfiguration(4, 0xff, 0, 1, vz.WithCustomVirtioProvider(provider))
			if err != nil {
				t.Fatal(err)
			}
			container := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
				config.SetEntropyDevicesVirtualMachineConfiguration(nil)
				return vz.WithCustomVirtioDevices(custom)(config)
			})
			defer container.Shutdown()
			if err := container.conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			session := container.NewSession(t)
			defer session.Close()
			output, err := session.Output("timeout 5 dd if=/dev/hwrng bs=64 count=8 iflag=fullblock status=none")
			if err != nil {
				t.Fatalf("read guest entropy: %v", err)
			}
			if !bytes.Equal(output, bytes.Repeat([]byte{0xa5}, 512)) {
				t.Fatalf("got %x, want 512 bytes of a5", output)
			}
		})
	}
}

type customVirtioGuestDelegate struct {
	customVirtioTestDelegate
	stopped chan *vz.CustomVirtioDevice
}

func (d *customVirtioGuestDelegate) WillStop(_ vz.CustomVirtioContext, device *vz.CustomVirtioDevice) {
	select {
	case d.stopped <- device:
	default:
	}
}

func TestCustomVirtioGuestLifecycle(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	delegate := &customVirtioGuestDelegate{
		customVirtioTestDelegate: customVirtioTestDelegate{
			created: make(chan *vz.CustomVirtioDevice, 2),
		},
		stopped: make(chan *vz.CustomVirtioDevice, 2),
	}
	provider, err := vz.NewCustomVirtioDeviceDelegateProvider(delegate)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	otherProvider, err := vz.NewCustomVirtioDeviceDelegateProvider(delegate)
	if err != nil {
		t.Fatal(err)
	}
	defer otherProvider.Close()
	var devices []*vz.CustomVirtioDeviceConfiguration
	for _, provider := range []*vz.CustomVirtioDeviceDelegateProvider{provider, otherProvider} {
		custom, err := vz.NewCustomVirtioDeviceConfiguration(0xff, 0xff, 0, 1, vz.WithCustomVirtioProvider(provider))
		if err != nil {
			t.Fatal(err)
		}
		devices = append(devices, custom)
	}
	container := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
		return vz.WithCustomVirtioDevices(devices...)(config)
	})
	defer container.Shutdown()
	created := make(map[uint64]*vz.CustomVirtioDevice)
	for range devices {
		select {
		case device := <-delegate.created:
			if device.ID() == 0 || created[device.ID()] != nil {
				t.Fatalf("device ID = %d, want a unique nonzero ID", device.ID())
			}
			created[device.ID()] = device
		case <-time.After(5 * time.Second):
			t.Fatal("custom device creation was not reported")
		}
	}
	if err := container.Shutdown(); err != nil {
		t.Fatal(err)
	}
	for range devices {
		select {
		case device := <-delegate.stopped:
			if created[device.ID()] == nil {
				t.Fatalf("stop reported unknown device ID %d", device.ID())
			}
			delete(created, device.ID())
		case <-time.After(5 * time.Second):
			t.Fatal("custom device stop was not reported")
		}
	}
}
