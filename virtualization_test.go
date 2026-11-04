package vz_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/testhelper"
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
		256*1024*1024,
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
	conn net.Conn
}

const (
	guestSetupTimeout   = 30 * time.Second
	guestSessionTimeout = 30 * time.Second
	guestStopTimeout    = 10 * time.Second
	guestConnectTimeout = 2 * time.Second
	guestRetryInterval  = 200 * time.Millisecond
)

func (c *Container) NewSession(t *testing.T) *testhelper.Session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), guestSessionTimeout)
	session, err := testhelper.NewSshSession(ctx, c.Client, c.conn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		session.Close()
		cancel()
	})
	return session
}

func (c *Container) Shutdown() error {
	if c.Client != nil {
		defer c.Client.Close()
	}
	if c.conn != nil {
		defer c.conn.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), guestStopTimeout)
	defer cancel()
	changed := make(chan vz.VirtualMachineState, 1)
	stopStateChanged, err := c.StateChangedNotify(changed)
	if err != nil {
		return err
	}
	defer stopStateChanged()
	requested := false
	for {
		state := c.State()
		if state == vz.VirtualMachineStateStopped || state == vz.VirtualMachineStateError {
			return nil
		}
		if !requested {
			switch {
			case !vz.Available(12) && c.CanStop():
				if err := vz.StopVirtualMachineContext(ctx, c.VirtualMachine); err != nil {
					return fmt.Errorf("stop virtual machine: %w", err)
				}
				requested = true
			case c.CanRequestStop():
				ok, err := c.RequestStop()
				if err != nil || !ok {
					return fmt.Errorf("request guest shutdown: accepted=%v, error=%v", ok, err)
				}
				requested = true
			case state == vz.VirtualMachineStateRunning && c.Client != nil:
				session, err := testhelper.NewSshSession(ctx, c.Client, c.conn)
				if err != nil {
					return err
				}
				defer session.Close()
				command := struct {
					Command string
				}{
					Command: "poweroff",
				}
				if _, err := session.SendRequest("exec", false, ssh.Marshal(command)); err != nil {
					return fmt.Errorf("request guest poweroff: %w", err)
				}
				requested = true
			}
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("wait for guest shutdown: %w", ctx.Err())
		}
	}
}

func getFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func newVirtualizationMachine(
	t *testing.T,
	configs ...func(*vz.VirtualMachineConfiguration) error,
) *Container {
	t.Helper()
	container, err := setupVirtualizationMachine(t, testhelper.NewSshConfig("root", "passwd"), configs...)
	if err != nil {
		t.Fatal(err)
	}
	return container
}

func setupVirtualizationMachine(
	t *testing.T,
	sshConfig *ssh.ClientConfig,
	configs ...func(*vz.VirtualMachineConfiguration) error,
) (*Container, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), guestSetupTimeout)
	defer cancel()
	port, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("resolve vsock port: %w", err)
	}
	bootLoader, err := vz.NewLinuxBootLoader(
		"./testdata/Image",
		vz.WithCommandLine(fmt.Sprintf("console=hvc0 vsock_port=%d", port)),
		vz.WithInitrd("./testdata/initramfs.cpio.gz"),
	)
	if err != nil {
		return nil, err
	}
	config, err := setupConfiguration(bootLoader)
	if err != nil {
		return nil, err
	}
	for _, setConfig := range configs {
		if err := setConfig(config); err != nil {
			return nil, err
		}
	}
	if valid, err := config.Validate(); err != nil || !valid {
		return nil, fmt.Errorf("validate guest configuration: valid=%v, error=%v", valid, err)
	}
	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		return nil, err
	}
	container := &Container{
		VirtualMachine: vm,
	}
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			t.Error(err)
		}
	})
	sockets := vm.SocketDevices()
	if len(sockets) != 1 {
		return container, fmt.Errorf("got %d socket devices, want 1", len(sockets))
	}
	if !vm.CanStart() {
		return container, errors.New("guest cannot start")
	}
	if err := vz.StartVirtualMachineContext(ctx, vm); err != nil {
		return container, err
	}
	if err := waitUntilStateContext(ctx, vm, vz.VirtualMachineStateRunning); err != nil {
		return container, err
	}
	if vz.Available(12) {
		// macOS 11 can leave an early vsock request pending instead of rejecting it.
		if err := waitGuestRetry(ctx, 5*time.Second); err != nil {
			return container, err
		}
	}
	for {
		attempt, cancel := context.WithTimeout(ctx, guestConnectTimeout)
		conn, err := sockets[0].ConnectContext(attempt, uint32(port))
		if err == nil {
			container.conn = conn
			deadline, _ := attempt.Deadline()
			if err = conn.SetDeadline(deadline); err == nil {
				container.Client, err = testhelper.NewSshClient(conn, ":22", sshConfig)
			}
			if err == nil {
				err = conn.SetDeadline(time.Time{})
				cancel()
				return container, err
			}
			conn.Close()
			container.conn = nil
		}
		attemptErr := attempt.Err()
		cancel()
		if err := ctx.Err(); err != nil {
			return container, fmt.Errorf("set up guest SSH: %w", err)
		}
		var nativeError *vz.NSError
		retry := errors.Is(attemptErr, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
		if errors.As(err, &nativeError) && nativeError.Code == int(syscall.ECONNRESET) {
			retry = true
		}
		if !retry {
			return container, err
		}
		if err := waitGuestRetry(ctx, guestRetryInterval); err != nil {
			return container, err
		}
	}
}

func waitGuestRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitState(t *testing.T, wait time.Duration, vm *vz.VirtualMachine, want vz.VirtualMachineState) {
	t.Helper()
	if err := waitUntilState(wait, vm, want); err != nil {
		t.Fatal(err)
	}
}

var ErrErrorState = fmt.Errorf("error state")

func waitUntilState(wait time.Duration, vm *vz.VirtualMachine, want vz.VirtualMachineState) error {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	return waitUntilStateContext(ctx, vm, want)
}

func waitUntilStateContext(ctx context.Context, vm *vz.VirtualMachine, want vz.VirtualMachineState) error {
	changed := make(chan vz.VirtualMachineState, 1)
	stopStateChanged, err := vm.StateChangedNotify(changed)
	if err != nil {
		return err
	}
	defer stopStateChanged()
	for {
		got := vm.State()
		if got == want {
			return nil
		}
		if got == vz.VirtualMachineStateError {
			return ErrErrorState
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("wait for state %s: %w", want, ctx.Err())
		}
	}
}

func TestRun(t *testing.T) {
	container := newVirtualizationMachine(t,
		func(vmc *vz.VirtualMachineConfiguration) error {
			return setupConsoleConfig(vmc)
		},
	)

	vm := container.VirtualMachine

	if got := vm.State(); vz.VirtualMachineStateRunning != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateRunning, got)
	}
	if got := vm.CanPause(); !got {
		t.Fatal("want CanPause is true")
	}
	changed := make(chan vz.VirtualMachineState, 8)
	stopStateChanged, err := vm.StateChangedNotify(changed)
	if err != nil {
		t.Fatal(err)
	}
	defer stopStateChanged()
	if err := vm.Pause(); err != nil {
		t.Fatal(err)
	}

	timeout := 5 * time.Second
	select {
	case <-changed:
	default:
		t.Fatal("pause did not notify state change")
	}
	waitState(t, timeout, vm, vz.VirtualMachineStatePaused)

	if got := vm.State(); vz.VirtualMachineStatePaused != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStatePaused, got)
	}
	if got := vm.CanResume(); !got {
		t.Fatal("want CanPause is true")
	}
	for len(changed) > 0 {
		<-changed
	}
	if err := vm.Resume(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-changed:
	default:
		t.Fatal("resume did not notify state change")
	}
	waitState(t, timeout, vm, vz.VirtualMachineStateRunning)

	if got := vm.CanRequestStop(); !got {
		t.Fatal("want CanRequestStop is true")
	}
	if success, err := vm.RequestStop(); !success || err != nil {
		t.Error(success, err)
		return
	}

	waitState(t, 5*time.Second, vm, vz.VirtualMachineStateStopped)

	if got := vm.State(); vz.VirtualMachineStateStopped != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateStopped, got)
	}
}

func TestStop(t *testing.T) {
	if vz.Available(12) {
		t.Skip("Stop is supported from macOS 12")
	}

	container := newVirtualizationMachine(t)

	vm := container.VirtualMachine

	if got := vm.CanStop(); !got {
		t.Fatal("want CanRequestStop is true")
	}
	stops := make(chan struct{}, 1)
	stopGuestDidStop, err := vm.GuestDidStopNotify(stops)
	if err != nil {
		t.Fatal(err)
	}
	defer stopGuestDidStop()
	if err := vm.Stop(); err != nil {
		t.Fatal(err)
	}

	timeout := 3 * time.Second
	if err := waitUntilState(timeout, vm, vz.VirtualMachineStateStopped); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stops:
		t.Fatal("host Stop reported a guest shutdown")
	default:
	}
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
		{
			state: vz.VirtualMachineStateSaving,
			want:  "VirtualMachineStateSaving",
		},
		{
			state: vz.VirtualMachineStateRestoring,
			want:  "VirtualMachineStateRestoring",
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
	if err := waitUntilState(timeout, vm, vz.VirtualMachineStatePaused); err != nil {
		t.Fatal(err)
	}

	if got := vm.State(); vz.VirtualMachineStatePaused != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStatePaused, got)
	}
	if got := vm.CanResume(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Resume(); err != nil {
		t.Fatal(err)
	}

	if err := waitUntilState(timeout, vm, vz.VirtualMachineStateRunning); err != nil {
		t.Fatal(err)
	}

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
	sshSession := container.NewSession(t)
	defer sshSession.Close()
	sshSession.Run("poweroff")
	waitState(t, timeout, vm, vz.VirtualMachineStateStopped)

	if got := vm.State(); vz.VirtualMachineStateStopped != got {
		t.Fatalf("want state %v but got %v", vz.VirtualMachineStateStopped, got)
	}
}
