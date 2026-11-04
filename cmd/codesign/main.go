package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var entitlements = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.virtualization</key>
	<true/>
</dict>
</plist>`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	options, args, err := parseArguments(os.Args[1:])
	if err != nil {
		return err
	}
	diagnostics, err := openDiagnosticLog(options, args[0])
	if err != nil {
		return err
	}
	if diagnostics != nil {
		defer diagnostics.Close()
	}
	f, err := os.CreateTemp("", "*.entitlements")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name()) // clean up

	if _, err := f.WriteString(entitlements); err != nil {
		return fmt.Errorf("failed to write entitlements content: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	cmd := exec.Command("codesign", "--entitlements", f.Name(), "-s", "-", args[0])
	cmd.Stderr = os.Stderr
	if options.dir != "" {
		diagnosticLog(diagnostics, args[0], 0, "signing begin")
	}
	err = cmd.Run()
	if options.dir != "" {
		diagnosticLog(diagnostics, args[0], 0, "signing end exit=%v", err)
	}
	if err != nil {
		return fmt.Errorf("failed to do codesign(%q): %w",
			strings.Join(
				[]string{
					"codesign", "--entitlements", f.Name(), "-s", "-", args[0],
				},
				" ",
			),
			err,
		)
	}
	testcmd := exec.Command(args[0], args[1:]...)
	testcmd.Stdout = os.Stdout
	testcmd.Stderr = os.Stderr
	testcmd.Stdin = os.Stdin

	return runTest(testcmd, options, diagnostics)
}
