package vz_test

import (
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v2"
	"golang.org/x/crypto/ssh"
)

func setupConsoleConfig(config *vz.VirtualMachineConfiguration) error {
	serialPortAttachment, err := vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stdout)
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

	sshConfig := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password("passwd")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	time.Sleep(3 * time.Second)

	clientCh := make(chan *ssh.Client, 1)
	socketDevice.ConnectToPort(2222, func(vsockConn *vz.VirtioSocketConnection, err error) {
		if err != nil {
			t.Errorf("failed to connect vsock: %v", err)
			return
		}

		sshClient, err := newSshClient(vsockConn, ":22", sshConfig)
		if err != nil {
			vsockConn.Close()
			t.Errorf("failed to create a new ssh client: %v", err)
			return
		}
		clientCh <- sshClient
		close(clientCh)
	})

	sshClient := <-clientCh

	return &Container{
		VirtualMachine: vm,
		Client:         sshClient,
	}
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

func newSshClient(conn net.Conn, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func setKeepAlive(t *testing.T, session *ssh.Session) {
	go func() {
		for range time.Tick(5 * time.Second) {
			_, err := session.SendRequest("keepalive@codehex.vz", true, nil)
			if err != nil && err != io.EOF {
				t.Logf("failed to send keep-alive request: %v", err)
				return
			}
		}
	}()
}

func TestRun(t *testing.T) {
	container := newVirtualizationMachine(t, func(vmc *vz.VirtualMachineConfiguration) error {
		return setupConsoleConfig(vmc)
	})
	sshSession, err := container.Client.NewSession()
	if err != nil {
		t.Error(err)
		return
	}
	defer sshSession.Close()
	setKeepAlive(t, sshSession)

	vm := container.VirtualMachine

	if got := vm.CanPause(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Pause(); err != nil {
		t.Fatal(err)
	}

	timeout := 5 * time.Second
	waitState(t, timeout, vm, vz.VirtualMachineStatePausing)
	waitState(t, timeout, vm, vz.VirtualMachineStatePaused)

	if got := vm.CanResume(); !got {
		t.Fatal("want CanPause is true")
	}
	if err := vm.Resume(); err != nil {
		t.Fatal(err)
	}

	waitState(t, timeout, vm, vz.VirtualMachineStateResuming)
	waitState(t, timeout, vm, vz.VirtualMachineStateRunning)

	// TODO(codehex): I need to support
	// see: https://developer.apple.com/forums/thread/702160
	//
	// t.Logf("CanRequestStop: %t", vm.CanRequestStop())
	// if success, err := vm.RequestStop(); !success || err != nil {
	// 	t.Error(success, err)
	// 	return
	// }

	// waitState(t, 5*time.Second, vm, vz.VirtualMachineStateStopping)
	// waitState(t, 5*time.Second, vm, vz.VirtualMachineStateStopped)

	sshSession.Run("poweroff")

	waitState(t, 5*time.Second, vm, vz.VirtualMachineStateStopped)
}
