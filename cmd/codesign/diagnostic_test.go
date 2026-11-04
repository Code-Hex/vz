package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseArgumentsPreservesTestFlags(t *testing.T) {
	options, args, err := parseArguments([]string{"-diagnostics", "/tmp/diagnostics", "-diagnostic-after", "2s", "/tmp/vz.test", "-test.run=TestVM", "-test.timeout=5m"})
	if err != nil {
		t.Fatal(err)
	}
	if options.dir != "/tmp/diagnostics" || options.after != 2*time.Second {
		t.Fatalf("options = %+v", options)
	}
	if strings.Join(args, " ") != "/tmp/vz.test -test.run=TestVM -test.timeout=5m" {
		t.Fatalf("test arguments = %q", args)
	}
	options, args, err = parseArguments([]string{"/tmp/vz.test", "-test.v"})
	if err != nil || options.dir != "" || options.after != 3*time.Minute || strings.Join(args, " ") != "/tmp/vz.test -test.v" {
		t.Fatalf("normal invocation = %+v, %q, %v", options, args, err)
	}
}

func TestRunTestNormalExitStopsDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
	}{
		{name: "success", code: 0},
		{name: "failure", code: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			output, err := os.Create(filepath.Join(t.TempDir(), "child.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			cmd := diagnosticChild("exit", strconv.Itoa(test.code))
			cmd.Stdout, cmd.Stderr = output, output
			options := diagnosticOptions{dir: dir, after: time.Hour}
			diagnostics, err := openDiagnosticLog(options, cmd.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer diagnostics.Close()
			err = runTest(cmd, options, diagnostics)
			if test.code == 0 && err != nil {
				t.Fatal(err)
			}
			if test.code != 0 {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != test.code {
					t.Fatalf("exit = %v, want code %d", err, test.code)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "sample") {
					t.Fatalf("normal child triggered capture: %s", entry.Name())
				}
			}
		})
	}
}

func TestRunTestCapturesActualBlockedChild(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "child.log")
	output, err := os.Create(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	cmd := diagnosticChild("block")
	cmd.Stdout, cmd.Stderr = output, output
	options := diagnosticOptions{dir: dir, after: 100 * time.Millisecond}
	diagnostics, err := openDiagnosticLog(options, cmd.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer diagnostics.Close()
	if err := runTest(cmd, options, diagnostics); err == nil {
		t.Fatal("blocked child exited successfully")
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	childPID := cmd.Process.Pid
	if childPID == os.Getpid() {
		t.Fatal("diagnostic target is the parent process")
	}
	for _, want := range []string{fmt.Sprintf("helper-pid=%d", childPID), "SIGQUIT", "diagnosticBlockedChild", "net.(*pipe).read"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("child output missing %q:\n%s", want, data)
		}
	}
	log, err := os.ReadFile(diagnostics.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), fmt.Sprintf("child_pid=%d", childPID)) || !strings.Contains(string(log), "sending SIGQUIT") {
		t.Fatalf("diagnostic log lacks actual child PID or SIGQUIT: %s", log)
	}
	statuses, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("*-%d-sample.status", childPID)))
	if err != nil || len(statuses) != 1 {
		t.Fatalf("sample statuses = %q, %v", statuses, err)
	}
	status, err := os.ReadFile(statuses[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), fmt.Sprintf("child_pid=%d", childPID)) || !strings.Contains(string(status), "exit=") {
		t.Fatalf("sample status lacks target or exit: %s", status)
	}
	sampleStderr, err := os.ReadFile(strings.TrimSuffix(statuses[0], ".status") + ".stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("native sample status: %s; stderr: %s", status, sampleStderr)
}

func TestRunTestCollectsWithUnreadChildStderr(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	cmd := diagnosticChild("stderr-block")
	cmd.Stderr = writer
	options := diagnosticOptions{dir: t.TempDir(), after: 100 * time.Millisecond}
	diagnostics, err := openDiagnosticLog(options, cmd.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer diagnostics.Close()
	if err := runTest(cmd, options, diagnostics); err == nil {
		t.Fatal("blocked child exited successfully")
	}
	log, err := os.ReadFile(diagnostics.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"collecting stacks", "sending SIGQUIT", fmt.Sprintf("child_pid=%d", cmd.Process.Pid)} {
		if !strings.Contains(string(log), want) {
			t.Errorf("diagnostic log missing %q: %s", want, log)
		}
	}
	statuses, err := filepath.Glob(filepath.Join(options.dir, fmt.Sprintf("*-%d-sample.status", cmd.Process.Pid)))
	if err != nil || len(statuses) != 1 {
		t.Fatalf("sample statuses = %q, %v", statuses, err)
	}
}

func diagnosticChild(args ...string) *exec.Cmd {
	return exec.Command(os.Args[0], append([]string{"-test.run=^TestDiagnosticChild$", "-test.timeout=1m", "--"}, args...)...)
}

func TestDiagnosticChild(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "--" || len(os.Args) <= i+1 {
			continue
		}
		fmt.Printf("helper-pid=%d\n", os.Getpid())
		switch os.Args[i+1] {
		case "exit":
			code, err := strconv.Atoi(os.Args[i+2])
			if err != nil {
				t.Fatal(err)
			}
			os.Exit(code)
		case "block":
			diagnosticBlockedChild()
		case "stderr-block":
			_, _ = os.Stderr.Write(make([]byte, 2<<20))
			diagnosticBlockedChild()
		}
		return
	}
}

func diagnosticBlockedChild() {
	reader, writer := net.Pipe()
	defer reader.Close()
	defer writer.Close()
	_, _ = io.Copy(io.Discard, reader)
}
