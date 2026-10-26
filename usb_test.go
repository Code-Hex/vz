package vz

import (
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
)

func TestUSBDeviceUUID(t *testing.T) {
	if err := macOSAvailable(15); err != nil {
		t.Skip(err)
	}
	path := filepath.Join(t.TempDir(), "disk.img")
	if err := CreateDiskImage(path, 512); err != nil {
		t.Fatal(err)
	}
	attachment, err := NewDiskImageStorageDeviceAttachment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(attachment)

	t.Run("specified UUID and reusable option", func(t *testing.T) {
		const want = "DFA9B5EE-FF88-48D9-A66D-86B22062D816"
		option := WithUSBDeviceUUID(want)
		for range 2 {
			config, err := NewUSBMassStorageDeviceConfiguration(attachment, option)
			if err != nil {
				t.Fatal(err)
			}
			defer objc.Release(config)
			device, err := NewUSBMassStorageDevice(config)
			if err != nil {
				t.Fatal(err)
			}
			defer objc.Release(device)
			if got := device.UUID(); got != want {
				t.Fatalf("UUID() = %q, want %q", got, want)
			}
		}
	})

	t.Run("generated UUID", func(t *testing.T) {
		config, err := NewUSBMassStorageDeviceConfiguration(attachment)
		if err != nil {
			t.Fatal(err)
		}
		defer objc.Release(config)
		device, err := NewUSBMassStorageDevice(config)
		if err != nil {
			t.Fatal(err)
		}
		defer objc.Release(device)
		if got := device.UUID(); len(got) != 36 {
			t.Fatalf("UUID() = %q, want an automatically generated UUID", got)
		}
	})

	for _, tt := range []struct {
		name string
		uuid string
	}{
		{name: "empty"},
		{name: "malformed", uuid: "not-a-uuid"},
		{name: "NUL", uuid: "DFA9B5EE-FF88-48D9-A66D-86B22062D816\x00ignored"},
		{name: "invalid UTF-8", uuid: "\xff"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewUSBMassStorageDeviceConfiguration(attachment, WithUSBDeviceUUID(tt.uuid))
			if config != nil || err == nil {
				t.Fatalf("configuration = %v, error = %v, want nil and an error", config, err)
			}
		})
	}
}
