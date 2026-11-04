package vz_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

// A fixed buffer keeps the observer from allocating a large Go heap object.
var diagnosticStackBuffer [1 << 20]byte

func diagnosticStackControl(format string, args ...any) {
	file, err := os.OpenFile("/tmp/vz-316-stack-control.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%s pid=%d %s\n", time.Now().UTC().Format(time.RFC3339Nano), os.Getpid(), fmt.Sprintf(format, args...))
}

func TestMain(m *testing.M) {
	file, err := os.Create("/tmp/vz-316-goroutines.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	capture := func(sequence int) {
		diagnosticStackControl("capture=%d begin", sequence)
		count := runtime.Stack(diagnosticStackBuffer[:], true)
		fmt.Fprintf(file, "\n--- capture=%d utc=%s pid=%d bytes=%d buffer_full=%v ---\n", sequence,
			time.Now().UTC().Format(time.RFC3339Nano), os.Getpid(), count, count == len(diagnosticStackBuffer))
		_, err := file.Write(diagnosticStackBuffer[:count])
		diagnosticStackControl("capture=%d end bytes=%d error=%v", sequence, count, err)
	}
	capture(0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		// Keep sampling until the 30-second watchdog can fire.
		for sequence := 1; sequence <= 60; sequence++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				capture(sequence)
			}
		}
	}()
	status := m.Run()
	diagnosticStackControl("test run exited status=%d", status)
	cancel()
	select {
	case <-done:
		file.Close()
		diagnosticStackControl("collector joined")
	case <-time.After(2 * time.Second):
		diagnosticStackControl("collector join timed out")
	}
	os.Exit(status)
}
