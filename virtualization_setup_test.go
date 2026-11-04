package vz_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/testhelper"
)

func TestVirtualMachineSetupFailureCleanup(t *testing.T) {
	var container *Container
	if !t.Run("authentication failure", func(t *testing.T) {
		var err error
		container, err = setupVirtualizationMachine(t, testhelper.NewSshConfig("root", "wrong-password"))
		if err == nil || !strings.Contains(err.Error(), "unable to authenticate") {
			t.Fatalf("setup error = %v, want an authentication failure", err)
		}
		if container == nil || container.State() != vz.VirtualMachineStateRunning {
			t.Fatal("setup did not reach a running guest")
		}
	}) {
		return
	}
	if got := container.State(); got != vz.VirtualMachineStateStopped {
		t.Fatalf("guest state after setup cleanup = %s, want stopped", got)
	}
}

func TestGuestCommandDeadlineCleanup(t *testing.T) {
	container := newVirtualizationMachine(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	session, err := testhelper.NewSshSession(ctx, container.Client, container.conn)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Run("sleep 5"); err == nil {
		t.Fatal("command completed without reaching its deadline")
	}
	session.Close()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("session context error = %v, want deadline exceeded", ctx.Err())
	}
	if err := container.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if got := container.State(); got != vz.VirtualMachineStateStopped {
		t.Fatalf("guest state after command timeout cleanup = %s, want stopped", got)
	}
}
