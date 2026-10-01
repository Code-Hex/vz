//go:build darwin && arm64

package vz

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

func TestNativeBridgeInvalidRestoreImageCompletes(t *testing.T) {
	if macOSAvailable(12) != nil {
		t.Skip("restore images require macOS 12")
	}
	runtime.GC()
	vzbridge.Drain()
	before := nativeCallbacks.next.Load()
	path := filepath.Join(t.TempDir(), "invalid.ipsw")
	if err := os.WriteFile(path, []byte("not a restore image"), 0600); err != nil {
		t.Fatal(err)
	}
	type result struct {
		image *MacOSRestoreImage
		err   error
	}
	completed := make(chan result, 1)
	go func() {
		image, err := LoadMacOSRestoreImageFromPath(path)
		completed <- result{image, err}
	}()
	select {
	case got := <-completed:
		if got.image != nil || got.err == nil {
			t.Fatalf("invalid restore image returned %+v", got)
		}
		var nativeError *NSError
		if !errors.As(got.err, &nativeError) || nativeError.Domain == "" || nativeError.LocalizedDescription == "" {
			t.Fatalf("restore image error fields were lost: %#v", got.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("restore image failure did not complete")
	}
	if got := nativeBridgeCallbackCountAfter(before); got != 0 {
		t.Fatalf("callback count after failed restore image = %d, want 0", got)
	}
}
