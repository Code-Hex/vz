//go:build darwin

package guiexample

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	vz "github.com/Code-Hex/vz/v4"
)

// Guest owns a virtual machine and its backing resources.
type Guest struct {
	VM        *vz.VirtualMachine
	Bootable  bool
	serial    *os.File
	directory string
}

// OpenGuest creates a bootable guest or an unstarted display preview.
func OpenGuest(opts Options) (*Guest, error) {
	g := &Guest{
		Bootable: opts.iso != "",
	}
	var err error
	g.directory, err = os.MkdirTemp("", "vz-native-gui-*")
	if err != nil {
		return nil, err
	}
	if opts.serialLog != "" {
		g.serial, err = os.Create(opts.serialLog)
		if err != nil {
			return nil, errors.Join(err, g.Close())
		}
	}
	g.VM, err = newMachine(opts, g.serial, filepath.Join(g.directory, "efi"))
	if err != nil {
		return nil, errors.Join(err, g.Close())
	}
	return g, nil
}

// Close stops the guest and releases its backing resources.
// Close native views and wait for VM operations before calling Close.
func (g *Guest) Close() error {
	var err error
	if g.VM != nil && g.VM.CanStop() {
		if err = g.VM.Stop(); err != nil {
			return err
		}
	}
	if g.serial != nil {
		err = errors.Join(err, g.serial.Close())
		g.serial = nil
	}
	if g.directory != "" {
		err = errors.Join(err, os.RemoveAll(g.directory))
		g.directory = ""
	}
	return err
}

func newMachine(opts Options, serial *os.File, storePath string) (*vz.VirtualMachine, error) {
	store, err := vz.NewEFIVariableStore(storePath, vz.WithCreatingEFIVariableStore())
	if err != nil {
		return nil, err
	}
	loader, err := vz.NewEFIBootLoader(vz.WithEFIVariableStore(store))
	if err != nil {
		return nil, err
	}
	config, err := vz.NewVirtualMachineConfiguration(loader, 4, 4*1024*1024*1024)
	if err != nil {
		return nil, err
	}
	graphics, err := vz.NewVirtioGraphicsDeviceConfiguration()
	if err != nil {
		return nil, err
	}
	scanout, err := vz.NewVirtioGraphicsScanoutConfiguration(1024, 768)
	if err != nil {
		return nil, err
	}
	graphics.SetScanouts(scanout)
	config.SetGraphicsDevicesVirtualMachineConfiguration([]vz.GraphicsDeviceConfiguration{graphics})
	keyboard, err := vz.NewUSBKeyboardConfiguration()
	if err != nil {
		return nil, err
	}
	config.SetKeyboardsVirtualMachineConfiguration([]vz.KeyboardConfiguration{keyboard})
	pointing, err := vz.NewUSBScreenCoordinatePointingDeviceConfiguration()
	if err != nil {
		return nil, err
	}
	config.SetPointingDevicesVirtualMachineConfiguration([]vz.PointingDeviceConfiguration{pointing})
	entropy, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, err
	}
	config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{entropy})
	if serial != nil {
		attachment, err := vz.NewFileHandleSerialPortAttachment(nil, serial)
		if err != nil {
			return nil, err
		}
		port, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(attachment)
		if err != nil {
			return nil, err
		}
		config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{port})
	}
	storage := make([]vz.StorageDeviceConfiguration, 0, 2)
	if opts.iso != "" {
		attachment, err := vz.NewDiskImageStorageDeviceAttachment(opts.iso, true)
		if err != nil {
			return nil, err
		}
		media, err := vz.NewUSBMassStorageDeviceConfiguration(attachment)
		if err != nil {
			return nil, err
		}
		storage = append(storage, media)
	}
	if opts.disk != "" {
		attachment, err := vz.NewDiskImageStorageDeviceAttachment(opts.disk, false)
		if err != nil {
			return nil, err
		}
		disk, err := vz.NewVirtioBlockDeviceConfiguration(attachment)
		if err != nil {
			return nil, err
		}
		storage = append(storage, disk)
	}
	config.SetStorageDevicesVirtualMachineConfiguration(storage)
	if valid, err := config.Validate(); err != nil || !valid {
		return nil, fmt.Errorf("invalid VM configuration: %v", err)
	}
	return vz.NewVirtualMachine(config)
}
