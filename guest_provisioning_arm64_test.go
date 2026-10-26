//go:build darwin && arm64

package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestGuestProvisioningStartOptions(t *testing.T) {
	if macOSAvailable(27) != nil {
		t.Skip("requires macOS 27")
	}
	options, err := NewMacGuestProvisioningOptions("Guest User", "vzuser", "guest-password", WithGuestAutomaticLogin(true), WithGuestRemoteLogin(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range [][]VirtualMachineStartOption{
		{WithStartUpFromMacOSRecovery(true), WithGuestProvisioningOptions(options)},
		{WithGuestProvisioningOptions(options), WithStartUpFromMacOSRecovery(true)},
	} {
		start := &virtualMachineStartOptions{}
		for _, opt := range opts {
			if err := opt(start); err != nil {
				t.Fatal(err)
			}
		}
		native := start.macOSVirtualMachineStartOptionsPtr
		if !vzbridge.VZMacOSVirtualMachineStartOptions_StartUpFromMacOSRecovery(native) {
			t.Fatal("recovery option was lost")
		}
		provisioning := vzbridge.VZMacOSVirtualMachineStartOptions_GuestProvisioningOptions(native)
		if provisioning == nil {
			t.Fatal("provisioning option was lost")
		}
		if got := objcutil.String(vzbridge.VZMacGuestProvisioningOptions_Username(provisioning)); got != "vzuser" {
			t.Fatalf("username = %q", got)
		}
		if !vzbridge.VZMacGuestProvisioningOptions_LogsInAutomatically(provisioning) || !vzbridge.VZMacGuestProvisioningOptions_EnablesRemoteLogin(provisioning) {
			t.Fatal("guest options were lost")
		}
	}
}

func TestGuestProvisioningInvalidAccount(t *testing.T) {
	if macOSAvailable(27) != nil {
		t.Skip("requires macOS 27")
	}
	for _, options := range []*MacGuestProvisioningOptions{nil, {}} {
		if err := WithGuestProvisioningOptions(options)(&virtualMachineStartOptions{}); err == nil {
			t.Fatal("accepted uninitialized guest provisioning options")
		}
	}
	options, err := NewMacGuestProvisioningOptions("Guest", "", "password")
	if err != nil {
		t.Fatal(err)
	}
	if err := WithGuestProvisioningOptions(options)(&virtualMachineStartOptions{}); err == nil {
		t.Fatal("accepted empty username")
	}
	for _, value := range []string{"secret\x00suffix", "\xff"} {
		if _, err := NewMacGuestProvisioningOptions("Guest", "vzuser", value); err == nil {
			t.Fatal("accepted unrepresentable password")
		}
	}
}
