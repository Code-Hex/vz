package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type diagnosticOptions struct {
	dir   string
	after time.Duration
}

func parseArguments(args []string) (diagnosticOptions, []string, error) {
	var options diagnosticOptions
	flags := flag.NewFlagSet("codesign", flag.ContinueOnError)
	flags.StringVar(&options.dir, "diagnostics", "", "directory for temporary test diagnostics")
	flags.DurationVar(&options.after, "diagnostic-after", 3*time.Minute, "delay before collecting test stacks")
	if err := flags.Parse(args); err != nil {
		return options, nil, err
	}
	if flags.NArg() == 0 {
		return options, nil, fmt.Errorf("missing test executable")
	}
	if options.after <= 0 {
		return options, nil, fmt.Errorf("diagnostic-after must be positive")
	}
	return options, flags.Args(), nil
}

func diagnosticLog(output io.Writer, binary string, pid int, format string, args ...any) {
	dir, _ := os.Getwd()
	fmt.Fprintf(output, "[vz-diagnostic] utc=%s wrapper_pid=%d child_pid=%d package_dir=%q binary=%q %s\n",
		time.Now().UTC().Format(time.RFC3339Nano), os.Getpid(), pid, dir, binary, fmt.Sprintf(format, args...))
}

func openDiagnosticLog(options diagnosticOptions, binary string) (*os.File, error) {
	if options.dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(options.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create diagnostics directory: %w", err)
	}
	path := filepath.Join(options.dir, fmt.Sprintf("%s-wrapper-%d.log", filepath.Base(binary), os.Getpid()))
	return os.Create(path)
}

func runTest(cmd *exec.Cmd, options diagnosticOptions, diagnostics io.Writer) error {
	if options.dir == "" {
		return cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	diagnosticLog(diagnostics, cmd.Path, cmd.Process.Pid, "test started diagnostic_after=%s", options.after)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchTest(ctx, cmd, options.after, diagnostics)
	}()
	err := cmd.Wait()
	cancel()
	<-done
	diagnosticLog(diagnostics, cmd.Path, cmd.Process.Pid, "test exited exit=%v", err)
	return err
}

func watchTest(ctx context.Context, cmd *exec.Cmd, after time.Duration, output io.Writer) {
	timer := time.NewTimer(after)
	defer timer.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			diagnosticLog(output, cmd.Path, cmd.Process.Pid, "heartbeat elapsed=%s", time.Since(started).Round(time.Second))
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			diagnosticLog(output, cmd.Path, cmd.Process.Pid, "collecting stacks elapsed=%s", time.Since(started).Round(time.Second))
			diagnosticLog(output, cmd.Path, cmd.Process.Pid, "sending SIGQUIT")
			if err := cmd.Process.Signal(syscall.SIGQUIT); err != nil {
				diagnosticLog(output, cmd.Path, cmd.Process.Pid, "SIGQUIT result=%v", err)
			}
			killTimer := time.NewTimer(10 * time.Second)
			defer killTimer.Stop()
			select {
			case <-ctx.Done():
			case <-killTimer.C:
				diagnosticLog(output, cmd.Path, cmd.Process.Pid, "SIGQUIT did not exit; killing child")
				if err := cmd.Process.Kill(); err != nil {
					diagnosticLog(output, cmd.Path, cmd.Process.Pid, "kill result=%v", err)
				}
			}
			return
		}
	}
}
