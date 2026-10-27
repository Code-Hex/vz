package vz

import "testing"

func TestVirtioConsoleMaximumPortCount(t *testing.T) {
	if macOSAvailable(13) != nil {
		t.Skip("requires macOS 13")
	}
	for _, tt := range []struct {
		name    string
		options []VirtioConsoleDeviceConfigurationOption
		want    uint32
	}{
		{
			name: "default",
			want: 4,
		},
		{
			name:    "explicit",
			options: []VirtioConsoleDeviceConfigurationOption{WithVirtioConsoleMaximumPortCount(8)},
			want:    8,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			console, err := NewVirtioConsoleDeviceConfiguration(tt.options...)
			if err != nil {
				t.Fatal(err)
			}
			port, err := NewVirtioConsolePortConfiguration()
			if err != nil {
				t.Fatal(err)
			}
			console.SetVirtioConsolePortConfiguration(3, port)
			if got := console.MaximumPortCount(); got != tt.want {
				t.Fatalf("maximum port count = %d, want %d", got, tt.want)
			}
			config := newTestConfig(t)
			config.SetConsoleDevicesVirtualMachineConfiguration([]ConsoleDeviceConfiguration{console})
			if ok, err := config.Validate(); !ok || err != nil {
				t.Fatalf("Validate() = %v, %v", ok, err)
			}
		})
	}
	if console, err := NewVirtioConsoleDeviceConfiguration(nil); console != nil || err == nil {
		t.Fatalf("nil option returned %v, %v", console, err)
	}
}
