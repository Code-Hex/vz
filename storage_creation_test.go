package vz_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v4"
)

func TestEFIVariableStoreOverwrite(t *testing.T) {
	if vz.Available(13) {
		t.Skip("requires macOS 13")
	}
	testStorageCreation(t, func(path string, allowOverwrite *bool) error {
		var options []vz.EFIVariableStoreCreationOption
		if allowOverwrite != nil {
			options = append(options, vz.WithEFIVariableStoreAllowOverwrite(*allowOverwrite))
		}
		_, err := vz.NewEFIVariableStore(path, vz.WithCreatingEFIVariableStore(options...))
		return err
	})
	t.Run("nil option", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "store")
		store, err := vz.NewEFIVariableStore(path, vz.WithCreatingEFIVariableStore(nil))
		if err == nil || store != nil {
			t.Fatalf("got (%v, %v), want error without a store", store, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid option created a file: %v", err)
		}
	})
}

func testStorageCreation(t *testing.T, create func(string, *bool) error) {
	t.Helper()
	allow := true
	deny := false
	for _, tt := range []struct {
		name           string
		existing       bool
		allowOverwrite *bool
	}{
		{name: "default overwrites", existing: true},
		{name: "allow overwrites", existing: true, allowOverwrite: &allow},
		{name: "deny preserves", existing: true, allowOverwrite: &deny},
		{name: "deny creates new", allowOverwrite: &deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "store")
			original := []byte("existing storage must survive")
			if tt.existing {
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := create(path, tt.allowOverwrite)
			preserve := tt.existing && tt.allowOverwrite != nil && !*tt.allowOverwrite
			if preserve && err == nil {
				t.Fatal("creation succeeded despite an existing file")
			}
			if !preserve && err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if preserve {
				if !bytes.Equal(contents, original) {
					t.Fatal("creation changed the existing file")
				}
			} else if len(contents) == 0 || bytes.Equal(contents, original) {
				t.Fatal("creation did not write initialized storage")
			}
		})
	}
}
