package vz_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

type customVirtioIteratorDelegate struct {
	customVirtioTestDelegate
	breakEarly bool
	errors     chan error
}

func (*customVirtioIteratorDelegate) DidCreateDevice(vz.CustomVirtioContext, *vz.CustomVirtioDevice) {
}

func (d *customVirtioIteratorDelegate) DidReceiveNotificationForQueue(_ vz.CustomVirtioContext, _ *vz.CustomVirtioDevice, queue *vz.VirtioQueue) {
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
	data := bytes.Repeat([]byte{0xa5}, int(min(element.WriteBuffersAvailableByteCount(), 64)))
	err := element.WriteBytes(data)
	if err == nil {
		err = element.ReturnToQueue()
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
			defer provider.Close()
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
			select {
			case delegateErr := <-delegate.errors:
				t.Fatal(delegateErr)
			default:
			}
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
