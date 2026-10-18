//go:build darwin

package vz

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNewGraphicWindowRejectsInvalidConfiguration(t *testing.T) {
	machine := &VirtualMachine{}
	for _, test := range []struct {
		name    string
		machine *VirtualMachine
		width   float64
		height  float64
		options []StartGraphicApplicationOption
	}{
		{name: "missing VM", width: 640, height: 480},
		{name: "zero width", machine: machine, height: 480},
		{name: "negative height", machine: machine, width: 640, height: -1},
		{name: "NaN width", machine: machine, width: math.NaN(), height: 480},
		{name: "infinite height", machine: machine, width: 640, height: math.Inf(1)},
		{name: "nil option", machine: machine, width: 640, height: 480, options: []StartGraphicApplicationOption{nil}},
	} {
		t.Run(test.name, func(t *testing.T) {
			window, err := NewGraphicWindow(test.machine, test.width, test.height, test.options...)
			if err == nil || window != nil {
				t.Fatalf("NewGraphicWindow = (%v, %v), want nil window and an error", window, err)
			}
		})
	}
	optionError := errors.New("invalid window option")
	_, err := NewGraphicWindow(machine, 640, 480, func(*startGraphicApplicationOptions) error {
		return optionError
	})
	if !errors.Is(err, optionError) {
		t.Fatalf("option error = %v, want %v", err, optionError)
	}
}

func TestStartGraphicApplicationRejectsInvalidWindows(t *testing.T) {
	if err := macOSAvailable(12); err != nil {
		t.Skip(err)
	}
	machine := &VirtualMachine{}
	first, err := NewGraphicWindow(machine, 640, 480)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewGraphicWindow(machine, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		windows []*GraphicWindow
		message string
	}{
		{name: "nil", windows: []*GraphicWindow{nil}, message: "valid windows"},
		{name: "zero value", windows: []*GraphicWindow{{}}, message: "valid windows"},
		{name: "duplicate VM", windows: []*GraphicWindow{first, second}, message: "same virtual machine twice"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := StartGraphicApplication(test.windows[0], test.windows[1:]...)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("StartGraphicApplication error = %v, want %q", err, test.message)
			}
		})
	}
}
