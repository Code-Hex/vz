package vz_test

import (
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/testhelper"
	"golang.org/x/crypto/ssh"
)

func setupConsoleConfig(config *vz.VirtualMachineConfiguration) error {
	serialPortAttachment, err := vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create file handle serial port attachment: %w", err)
	}
	consoleConfig, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialPortAttachment)
	if err != nil {
		return fmt.Errorf("failed to create a console device serial port config: %w", err)
	}
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{
		consoleConfig,
	})
	return nil
}

func setupNetworkConfig(config *vz.VirtualMachineConfiguration) error {
	natAttachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return fmt.Errorf("failed to create NAT network device attachment: %w", err)
	}
	networkConfig, err := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	if err != nil {
		return fmt.Errorf("failed to create a network device config: %w", err)
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{
		networkConfig,
	})
	mac, err := vz.NewRandomLocallyAdministeredMACAddress()
	if err != nil {
		return fmt.Errorf("failed to generate random MAC address: %w", err)
	}
	networkConfig.SetMACAddress(mac)
	return nil
}

func setupConfiguration(bootLoader vz.BootLoader) (*vz.VirtualMachineConfiguration, error) {
	config, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		1,
		512*1024*1024,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new virtual machine config: %w", err)
	}

	// entropy device
	entropyConfig, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create entropy device config: %w", err)
	}
	config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{
		entropyConfig,
	})

	// memory balloon device
	memoryBalloonDevice, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create memory balloon device config: %w", err)
	}
	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{
		memoryBalloonDevice,
	})

	// vsock device
	vsockDevice, err := vz.NewVirtioSocketDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to create virtio socket device config: %w", err)
	}
	config.SetSocketDevicesVirtualMachineConfiguration([]vz.SocketDeviceConfiguration{
		vsockDevice,
	})

	if err := setupNetworkConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

type Container struct {
	*vz.VirtualMachine
	*ssh.Client
}

func (c *Container) Close() error {
	return c.Client.Close()
}

func (c *Container) NewSession(t *testing.T) *ssh.Session {
	sshSession, err := c.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	testhelper.SetKeepAlive(t, sshSession)
	return sshSession
}

func newVirtualizationMachine(
	t *testing.T,
	configs ...func(*vz.VirtualMachineConfiguration) error,
) *Container {
	vmlinuz := "./testdata/Image"
	initramfs := "./testdata/initramfs.cpio.gz"
	bootLoader, err := vz.NewLinuxBootLoader(
		vmlinuz,
		vz.WithCommandLine("console=hvc0"),
		vz.WithInitrd(initramfs),
	)
	if err != nil {
		t.Fatal(err)
	}

	config, err := setupConfiguration(bootLoader)
	if err != nil {
		t.Fatal(err)
	}
	for _, setConfig := range configs {
		if err := setConfig(config); err != nil {
			t.Fatal(err)
		}
	}

	validated, err := config.Validate()
	if !validated || err != nil {
		t.Fatal(validated, err)
	}

	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	socketDevices := vm.SocketDevices()
	if len(socketDevices) != 1 {
		t.Fatalf("want the number of socket devices is 1 but got %d", len(socketDevices))
	}
	socketDevice := socketDevices[0]

	if canStart := vm.CanStart(); !canStart {
		t.Fatal("want CanStart is true")
	}

	if err := vm.Start(); err != nil {
		t.Fatal(err)
	}

	waitState(t, 3*time.Second, vm, vz.VirtualMachineStateStarting)
	waitState(t, 3*time.Second, vm, vz.VirtualMachineStateRunning)

	sshConfig := testhelper.NewSshConfig("root", "passwd")

	// Workaround for macOS 11
	//
	// This is a workaround. This version of the API does not immediately return an error and
	// does not seem to have a connection timeout set.
	if vz.Available(12) {
		time.Sleep(5 * time.Second)
	}

	max := 8
RETRY:
	for i := 1; ; i++ {
		conn, err := socketDevice.Connect(2222)
		if err != nil {
			var nserr *vz.NSError
			if !errors.As(err, &nserr) || i > max {
				t.Fatal(err)
			}
			if nserr.Code == int(syscall.ECONNRESET) {
				t.Logf("retry vsock connect: %d", i)
				time.Sleep(backOffDelay(i))
				continue RETRY
			}
			t.Fatalf("failed to connect vsock: %v", err)
		}

		t.Log("setup ssh client in container")

		initialized := make(chan struct{})
		retry := make(chan struct{})
		go func() {
			select {
			case <-initialized:
			case <-time.After(5 * time.Second):
				close(retry)
				t.Log("closed", conn.Close())
			}
		}()

		sshClient, err := testhelper.NewSshClient(conn, ":22", sshConfig)
		if err != nil {
			select {
			case <-retry:
				t.Log("retry because ssh handshake has been failed")
				continue RETRY
			default:
			}
			conn.Close()
			t.Fatalf("failed to create a new ssh client: %v", err)
		}

		close(initialized)

		t.Logf("container setup done")

		return &Container{
			VirtualMachine: vm,
			Client:         sshClient,
		}
	}
}

func backOffDelay(retryAttempts int) time.Duration {
	factor := 0.5
	delay := math.Exp2(float64(retryAttempts)) * factor
	return time.Duration(math.Min(delay, 10)) * time.Second
}

func waitState(t *testing.T, wait time.Duration, vm *vz.VirtualMachine, want vz.VirtualMachineState) {
	t.Helper()
	select {
	case got := <-vm.StateChangedNotify():
		if want != got {
			t.Fatalf("unexpected state want %d but got %d", want, got)
		}
	case <-time.After(wait):
		t.Fatal("failed to wait state changed notification")
	}
}

func TestRun(t *testing.T) {
	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			return setupConsoleConfig(vmc)
		},
	)
	defer container.Close()

	sshSession := container.NewSession(t)
	defer sshSession.Close()

	vm := container.VirtualMachine

	if got := vm.State(); vz.VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateRunning, got)
	}
	if got := vm.CanPause(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Pause(); err != nil {
		t.Fatal(err)
	}

	timeout := 5 * time.Second
	waitState(t, timeout, vm, vz.VirtualMachineStatePausing)
	waitState(t, timeout, vm, vz.VirtualMachineStatePaused)

	if got := vm.State(); vz.VirtualMachineStatePaused != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStatePaused, got)
	}
	if got := vm.CanResume(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Resume(); err != nil {
		t.Fatal(err)
	}

	waitState(t, timeout, vm, vz.VirtualMachineStateResuming)
	waitState(t, timeout, vm, vz.VirtualMachineStateRunning)

	if got := vm.CanRequestStop(); !got {
		t.Fatal("want CanRequestStop is true")
	}
	// TODO(codehex): I need to support
	// see: https://developer.apple.com/forums/thread/702160
	//
	// if success, err := vm.RequestStop(); !success || err != nil {
	// 	t.Error(success, err)
	// 	return
	// }

	// waitState(t, 5*time.Second, vm, vz.VirtualMachineStateStopping)
	// waitState(t, 5*time.Second, vm, vz.VirtualMachineStateStopped)

	sshSession.Run("poweroff")

	waitState(t, timeout, vm, vz.VirtualMachineStateStopped)

	if got := vm.State(); vz.VirtualMachineStateStopped != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateStopped, got)
	}
}

func TestStop(t *testing.T) {
	if vz.Available(12) {
		t.Skip("Stop is supported from macOS 12")
	}

	container := newVirtualizationMachine(t)
	defer container.Close()

	vm := container.VirtualMachine

	if got := vm.CanStop(); !got {
		t.Fatal("want CanRequestStop is true")
	}
	if err := vm.Stop(); err != nil {
		t.Fatal(err)
	}

	timeout := 3 * time.Second
	waitState(t, timeout, vm, vz.VirtualMachineStateStopping)
	waitState(t, timeout, vm, vz.VirtualMachineStateStopped)
}

func TestVirtualMachineStateString(t *testing.T) {
	cases := []struct {
		state vz.VirtualMachineState
		want  string
	}{
		{
			state: vz.VirtualMachineStateStopped,
			want:  "VirtualMachineStateStopped",
		},
		{
			state: vz.VirtualMachineStateRunning,
			want:  "VirtualMachineStateRunning",
		},
		{
			state: vz.VirtualMachineStatePaused,
			want:  "VirtualMachineStatePaused",
		},
		{
			state: vz.VirtualMachineStateError,
			want:  "VirtualMachineStateError",
		},
		{
			state: vz.VirtualMachineStateStarting,
			want:  "VirtualMachineStateStarting",
		},
		{
			state: vz.VirtualMachineStatePausing,
			want:  "VirtualMachineStatePausing",
		},
		{
			state: vz.VirtualMachineStateResuming,
			want:  "VirtualMachineStateResuming",
		},
		{
			state: vz.VirtualMachineStateStopping,
			want:  "VirtualMachineStateStopping",
		},
	}
	for _, tc := range cases {
		got := tc.state.String()
		if tc.want != got {
			t.Fatalf("want %q but got %q", tc.want, got)
		}
	}
}

func TestRunIssue124(t *testing.T) {
	if os.Getenv("TEST_ISSUE_124") != "1" {
		t.Skip()
	}
	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			return setupConsoleConfig(vmc)
		},
	)
	defer container.Close()

	sshSession := container.NewSession(t)
	defer sshSession.Close()

	vm := container.VirtualMachine

	if got := vm.State(); vz.VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateRunning, got)
	}
	if got := vm.CanPause(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Pause(); err != nil {
		t.Fatal(err)
	}

	timeout := 5 * time.Second
	waitState(t, timeout, vm, vz.VirtualMachineStatePausing)
	waitState(t, timeout, vm, vz.VirtualMachineStatePaused)

	if got := vm.State(); vz.VirtualMachineStatePaused != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStatePaused, got)
	}
	if got := vm.CanResume(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Resume(); err != nil {
		t.Fatal(err)
	}

	waitState(t, timeout, vm, vz.VirtualMachineStateResuming)
	waitState(t, timeout, vm, vz.VirtualMachineStateRunning)

	if got := vm.CanRequestStop(); !got {
		t.Fatal("want CanRequestStop is true")
	}

	ch := make(chan bool)
	vm.SetMachineStateFinalizer(func() {
		ch <- true
	})

	runtime.GC()
	select {
	case <-ch:
		t.Errorf("expected finalizer do not run")
	case <-time.After(4 * time.Minute):
	}

	runtime.GC()
	sshSession.Run("poweroff")
	waitState(t, timeout, vm, vz.VirtualMachineStateStopped)

	if got := vm.State(); vz.VirtualMachineStateStopped != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateStopped, got)
	}
}
