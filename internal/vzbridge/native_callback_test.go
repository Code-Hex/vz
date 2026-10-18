//go:build darwin

package vzbridge

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeCallbackWithoutReceiver(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "native-callback-cleanup")
	build := exec.CommandContext(ctx, "xcrun", "clang", "-fblocks", "-I.",
		"-framework", "Foundation", "-framework", "Virtualization",
		"-o", binary, "testdata/native_callback_cleanup.m")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native callback test: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("native callback test: %v\n%s", err, output)
	}
}
