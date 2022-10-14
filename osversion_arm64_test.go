//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestAvailableVersionArm64(t *testing.T) {
	majorVersionOnce = &nopDoer{}
	defer func() {
		majorVersion = 0
		majorVersionOnce = &sync.Once{}
	}()
	t.Run("macOS 12", func(t *testing.T) {
		majorVersion = 11
		cases := map[string]func() error{
			"NewMacOSBootLoader": func() error {
				_, err := NewMacOSBootLoader()
				return err
			},
			"NewMacGraphicsDeviceConfiguration": func() error {
				_, err := NewMacGraphicsDeviceConfiguration()
				return err
			},
			"NewMacGraphicsDisplayConfiguration": func() error {
				_, err := NewMacGraphicsDisplayConfiguration(0, 0, 0)
				return err
			},
			"NewMacPlatformConfiguration": func() error {
				_, err := NewMacPlatformConfiguration()
				return err
			},
			"NewMacHardwareModelWithData": func() error {
				_, err := NewMacHardwareModelWithData(nil)
				return err
			},
			"NewMacMachineIdentifierWithData": func() error {
				_, err := NewMacMachineIdentifierWithData(nil)
				return err
			},
			"NewMacMachineIdentifier": func() error {
				_, err := NewMacMachineIdentifier()
				return err
			},
			"NewMacAuxiliaryStorage": func() error {
				_, err := NewMacAuxiliaryStorage("")
				return err
			},
			"FetchLatestSupportedMacOSRestoreImage": func() error {
				_, err := FetchLatestSupportedMacOSRestoreImage(context.Background(), "")
				return err
			},
			"LoadMacOSRestoreImageFromPath": func() error {
				_, err := LoadMacOSRestoreImageFromPath("")
				return err
			},
			"NewMacOSInstaller": func() error {
				_, err := NewMacOSInstaller(nil, "")
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
}
