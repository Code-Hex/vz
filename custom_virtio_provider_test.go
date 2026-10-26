package vz_test

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func newCustomVirtioTestProvider(t *testing.T) *vz.CustomVirtioDeviceDelegateProvider {
	t.Helper()
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	provider, err := vz.NewCustomVirtioDeviceDelegateProvider(&customVirtioTestDelegate{
		created: make(chan *vz.CustomVirtioDevice, 4),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { provider.Close() })
	return provider
}

func TestCustomVirtioProviderDuplicate(t *testing.T) {
	for _, name := range []string{"same-device", "separate-devices"} {
		t.Run(name, func(t *testing.T) {
			provider := newCustomVirtioTestProvider(t)
			config := customVirtioMachineConfiguration(t, provider)
			devices, err := config.CustomVirtioDevices()
			if err != nil {
				t.Fatal(err)
			}
			other := devices[0]
			if name == "separate-devices" {
				providerCopy := *provider
				other, err = vz.NewCustomVirtioDeviceConfiguration(4, 0xff, 0, 1, vz.WithCustomVirtioProvider(&providerCopy))
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := vz.WithCustomVirtioDevices(devices[0], other)(config); err != nil {
				t.Fatal(err)
			}
			if _, err := vz.NewVirtualMachine(config); err == nil {
				t.Fatal("duplicate provider was accepted")
			}
			if err := vz.WithCustomVirtioDevices(devices[0])(config); err != nil {
				t.Fatal(err)
			}
			machine, err := vz.NewVirtualMachine(config)
			if err != nil {
				t.Fatalf("rejected configuration consumed the provider: %v", err)
			}
			runtime.KeepAlive(machine)
		})
	}
}

func TestCustomVirtioProviderReuse(t *testing.T) {
	provider := newCustomVirtioTestProvider(t)
	config := customVirtioMachineConfiguration(t, provider)
	machine, err := vz.NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(machine)
	if _, err := vz.NewVirtualMachine(config); err == nil {
		t.Fatal("provider was reused by a second virtual machine")
	}
	devices, err := config.CustomVirtioDevices()
	if err != nil {
		t.Fatal(err)
	}
	deviceCopy := *devices[0]
	configCopy := *config
	if err := vz.WithCustomVirtioDevices(&deviceCopy)(&configCopy); err != nil {
		t.Fatal(err)
	}
	if _, err := vz.NewVirtualMachine(&configCopy); err == nil {
		t.Fatal("copied configuration bypassed provider reuse check")
	}
}

func TestCustomVirtioProviderConcurrentCreation(t *testing.T) {
	provider := newCustomVirtioTestProvider(t)
	config := customVirtioMachineConfiguration(t, provider)
	type result struct {
		machine *vz.VirtualMachine
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			machine, err := vz.NewVirtualMachine(config)
			results <- result{
				machine: machine,
				err:     err,
			}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if (first.err == nil) == (second.err == nil) {
		t.Fatalf("want one creation to succeed, got %v and %v", first.err, second.err)
	}
	runtime.KeepAlive(first.machine)
	runtime.KeepAlive(second.machine)
}
