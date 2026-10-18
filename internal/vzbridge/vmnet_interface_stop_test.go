//go:build darwin

package vzbridge

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestVMNetInterfaceStopFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "vmnet-interface-stop")
	build := exec.CommandContext(ctx, "xcrun", "clang", "-fblocks", "-I.",
		"-framework", "Foundation", "-framework", "vmnet",
		"-o", binary, "testdata/vmnet_interface_stop.m")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native stop test: %v\n%s", err, output)
	}
	output, err := exec.CommandContext(ctx, binary).CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 77 {
		t.Skip("vmnet interfaces require macOS 26 and its SDK")
	}
	if err != nil {
		t.Fatalf("native stop test: %v\n%s", err, output)
	}
}
