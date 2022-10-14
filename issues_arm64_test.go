//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"errors"
	"os"
	"testing"
)

func TestIssue43Arm64(t *testing.T) {
	const doesNotExists = "/non/existing/path"
	t.Run("does not throw NSInvalidArgumentException", func(t *testing.T) {
		cases := map[string]func() error{
			// This is also fixed issue #71
			"NewMacAuxiliaryStorage": func() error {
				_, err := NewMacAuxiliaryStorage(doesNotExists)
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
