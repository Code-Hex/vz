package vz

import (
	"os"
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
		cases := map[string]func(){
			"start handler": func() {
				m.Start(func(err error) { _ = err == nil })
			},
			"pause handler": func() {
				m.Pause(func(err error) { _ = err == nil })
			},
			"resume handler": func() {
				m.Resume(func(err error) { _ = err == nil })
			},
			"stop handler": func() {
				m.Stop(func(err error) { _ = err == nil })
			},
		}
		for name, run := range cases {
			t.Run(name, func(t *testing.T) {
				run()
			})
		}
	})
}

func TestIssue71(t *testing.T) {
	_, err := NewFileSerialPortAttachment("/non/existing/path", false)
	if err == nil {
		t.Error("NewFileSerialPortAttachment should have returned an error")
	}
}
