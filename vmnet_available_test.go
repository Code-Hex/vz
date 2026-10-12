package vz

import (
	"errors"
	"sync"
	"testing"
)

func TestVMNetAvailability(t *testing.T) {
	majorMinorVersionOnce.Do(func() {})
	previous := majorMinorVersion
	majorMinorVersion = 15
	defer func() { majorMinorVersion = previous; majorMinorVersionOnce = sync.Once{} }()
	if _, err := NewVMNetNetworkDeviceAttachment(nil); !errors.Is(err, ErrUnsupportedOSVersion) {
		t.Fatalf("error = %v, want unsupported OS", err)
	}
	if (*VMNetNetworkDeviceAttachment)(nil).Network() != nil {
		t.Fatal("old OS attachment getter returned a network")
	}
}
