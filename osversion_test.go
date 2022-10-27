package vz

import (
	"errors"
	"sync"
	"testing"
)

type nopDoer struct{}

func (*nopDoer) Do(func()) {}

func TestAvailableVersion(t *testing.T) {
	majorVersionOnce = &nopDoer{}
	defer func() {
		majorVersion = 0
		majorVersionOnce = &sync.Once{}
	}()

	t.Run("macOS 11", func(t *testing.T) {
		majorVersion = 10
		cases := map[string]func() error{
			"NewLinuxBootLoader": func() error {
				_, err := NewLinuxBootLoader("")
				return err
			},
			"NewVirtualMachineConfiguration": func() error {
				_, err := NewVirtualMachineConfiguration(nil, 0, 0)
				return err
			},
			"NewFileHandleSerialPortAttachment": func() error {
				_, err := NewFileHandleSerialPortAttachment(nil, nil)
				return err
			},
			"NewFileSerialPortAttachment": func() error {
				_, err := NewFileSerialPortAttachment("", false)
				return err
			},
			"NewVirtioConsoleDeviceSerialPortConfiguration": func() error {
				_, err := NewVirtioConsoleDeviceSerialPortConfiguration(nil)
				return err
			},
			"NewVirtioEntropyDeviceConfiguration": func() error {
				_, err := NewVirtioEntropyDeviceConfiguration()
				return err
			},
			"NewVirtioTraditionalMemoryBalloonDeviceConfiguration": func() error {
				_, err := NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
				return err
			},
			"NewNATNetworkDeviceAttachment": func() error {
				_, err := NewNATNetworkDeviceAttachment()
				return err
			},
			"NewBridgedNetworkDeviceAttachment": func() error {
				_, err := NewBridgedNetworkDeviceAttachment(nil)
				return err
			},
			"NewFileHandleNetworkDeviceAttachment": func() error {
				_, err := NewFileHandleNetworkDeviceAttachment(nil)
				return err
			},
			"NewVirtioNetworkDeviceConfiguration": func() error {
				_, err := NewVirtioNetworkDeviceConfiguration(nil)
				return err
			},
			"NewMACAddress": func() error {
				_, err := NewMACAddress(nil)
				return err
			},
			"NewRandomLocallyAdministeredMACAddress": func() error {
				_, err := NewRandomLocallyAdministeredMACAddress()
				return err
			},
			"NewVirtioSocketDeviceConfiguration": func() error {
				_, err := NewVirtioSocketDeviceConfiguration()
				return err
			},
			"(*VirtioSocketDevice).Listen": func() error {
				_, err := (*VirtioSocketDevice)(nil).Listen(1)
				return err
			},
			"NewDiskImageStorageDeviceAttachment": func() error {
				_, err := NewDiskImageStorageDeviceAttachment("", false)
				return err
			},
			"NewVirtioBlockDeviceConfiguration": func() error {
				_, err := NewVirtioBlockDeviceConfiguration(nil)
				return err
			},
			"NewVirtualMachine": func() error {
				_, err := NewVirtualMachine(nil)
				return err
			},
		}
		for name, fn := range cases {
			err := fn()
			if !errors.Is(err, ErrUnsupportedOSVersion) {
				t.Fatalf("unexpected error %v in %s", err, name)
			}
		}
	})

	t.Run("macOS 12", func(t *testing.T) {
		majorVersion = 11
		cases := map[string]func() error{
			"NewVirtioSoundDeviceConfiguration": func() error {
				_, err := NewVirtioSoundDeviceConfiguration()
				return err
			},
			"NewVirtioSoundDeviceHostInputStreamConfiguration": func() error {
				_, err := NewVirtioSoundDeviceHostInputStreamConfiguration()
				return err
			},
			"NewVirtioSoundDeviceHostOutputStreamConfiguration": func() error {
				_, err := NewVirtioSoundDeviceHostOutputStreamConfiguration()
				return err
			},
			"NewUSBKeyboardConfiguration": func() error {
				_, err := NewUSBKeyboardConfiguration()
				return err
			},
			"NewGenericPlatformConfiguration": func() error {
				_, err := NewGenericPlatformConfiguration()
				return err
			},
			"NewUSBScreenCoordinatePointingDeviceConfiguration": func() error {
				_, err := NewUSBScreenCoordinatePointingDeviceConfiguration()
				return err
			},
			"NewVirtioFileSystemDeviceConfiguration": func() error {
				_, err := NewVirtioFileSystemDeviceConfiguration("")
				return err
			},
			"NewSharedDirectory": func() error {
				_, err := NewSharedDirectory("", false)
				return err
			},
			"NewSingleDirectoryShare": func() error {
				_, err := NewSingleDirectoryShare(nil)
				return err
			},
			"NewMultipleDirectoryShare": func() error {
				_, err := NewMultipleDirectoryShare(nil)
				return err
			},
			"(*VirtualMachine).Stop": func() error {
				return (*VirtualMachine)(nil).Stop()
			},
			"(*VirtualMachine).StartGraphicApplication": func() error {
				return (*VirtualMachine)(nil).StartGraphicApplication(0, 0)
			},
		}
		for name, fn := range cases {
			err := fn()
			if !errors.Is(err, ErrUnsupportedOSVersion) {
				t.Fatalf("unexpected error %v in %s", err, name)
			}
		}
	})
}
