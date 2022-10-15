package vz

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestIssue50(t *testing.T) {
	f, err := os.CreateTemp("", "vmlinuz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	bootloader, err := NewLinuxBootLoader(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	config, err := NewVirtualMachineConfiguration(bootloader, 1, 1024*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := config.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("failed to validate config")
	}
	m, err := NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check for segmentation faults", func(t *testing.T) {
		cases := map[string]func() error{
			"start handler":  m.Start,
			"pause handler":  m.Pause,
			"resume handler": m.Resume,
			"stop handler":   m.Stop,
		}
		for name, run := range cases {
			t.Run(name, func(t *testing.T) {
				_ = run()
			})
		}
	})
}

func TestIssue43(t *testing.T) {
	const doesNotExists = "/non/existing/path"
	t.Run("does not throw NSInvalidArgumentException", func(t *testing.T) {
		t.Run("NewLinuxBootLoader", func(t *testing.T) {
			_, err := NewLinuxBootLoader(doesNotExists)
			if err == nil {
				t.Fatal("expected returns error")
			}
			if !strings.HasPrefix(err.Error(), "invalid linux kernel") {
				t.Error(err)
			}
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("want underlying error %q but got %q", os.ErrNotExist, err)
			}

			f, err := os.CreateTemp("", "vmlinuz")
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			_, err = NewLinuxBootLoader(f.Name(), WithInitrd(doesNotExists))
			if err == nil {
				t.Fatal("expected returns error")
			}
			if !strings.HasPrefix(err.Error(), "invalid initial RAM disk") {
				t.Error(err)
			}
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("want underlying error %q but got %q", os.ErrNotExist, err)
			}
		})

		cases := map[string]func() error{
			// This is also fixed issue #71
			"NewFileSerialPortAttachment": func() error {
				_, err := NewFileSerialPortAttachment(doesNotExists, false)
				return err
			},
			"NewSharedDirectory": func() error {
				_, err := NewFileSerialPortAttachment(doesNotExists, false)
				return err
			},
			"NewDiskImageStorageDeviceAttachment": func() error {
				_, err := NewDiskImageStorageDeviceAttachment(doesNotExists, false)
				return err
			},
		}
		for name, run := range cases {
			t.Run(name, func(t *testing.T) {
				err := run()
				if err == nil {
					t.Fatal("expected returns error")
				}
				if !errors.Is(err, os.ErrNotExist) {
					t.Errorf("want underlying error %q but got %q", os.ErrNotExist, err)
				}
			})
		}
	})
}
