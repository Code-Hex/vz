//go:build darwin && arm64

package vz_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestMacAuxiliaryStorageOverwrite(t *testing.T) {
	if vz.Available(12) {
		t.Skip("requires macOS 12")
	}
	// This model comes from Apple's restore image catalog. It contains only
	// format, platform, and minimum OS versions, not a machine identifier.
	modelData, err := hex.DecodeString(
		"62706c6973743030d30102030404055f101944617461526570726573656e7461" +
			"74696f6e56657273696f6e5f100f506c6174666f726d56657273696f6e5f1012" +
			"4d696e696d756d537570706f727465644f531002a3060707100c1000080f2b3d" +
			"5254585a00000000000001010000000000000008000000000000000000000000" +
			"0000005c",
	)
	if err != nil {
		t.Fatal(err)
	}
	model, err := vz.NewMacHardwareModelWithData(modelData)
	if err != nil {
		t.Fatal(err)
	}
	testStorageCreation(t, func(path string, allowOverwrite *bool) error {
		var options []vz.MacAuxiliaryStorageCreationOption
		if allowOverwrite != nil {
			options = append(options, vz.WithMacAuxiliaryStorageAllowOverwrite(*allowOverwrite))
		}
		_, err := vz.NewMacAuxiliaryStorage(path, vz.WithCreatingMacAuxiliaryStorage(model, options...))
		return err
	})
	t.Run("nil option", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "store")
		store, err := vz.NewMacAuxiliaryStorage(path, vz.WithCreatingMacAuxiliaryStorage(model, nil))
		if err == nil || store != nil {
			t.Fatalf("got (%v, %v), want error without storage", store, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid option created a file: %v", err)
		}
	})
}
