package guiexample

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// Options contains guest settings and native display preferences.
type Options struct {
	iso, serialLog, disk               string
	Autostart, CaptureKeys, AutoResize bool
}

func parseOptions(args []string) (Options, error) {
	var opts Options
	flags := flag.NewFlagSet("go-native-gui", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Select a framework: make run FRAMEWORK=mygo|fyne|wails|gio ARGS='...'")
		fmt.Fprintln(flags.Output(), "Build a framework: go build ./mygo (sign before running)")
		flags.PrintDefaults()
	}
	flags.StringVar(&opts.iso, "iso", "", "bootable Linux ISO matching the host architecture; omit for display preview")
	flags.StringVar(&opts.serialLog, "serial-log", "", "write guest serial output to this file")
	flags.StringVar(&opts.disk, "disk", "", "existing writable raw disk image")
	flags.BoolVar(&opts.Autostart, "autostart", false, "start after mounting the native display")
	flags.BoolVar(&opts.CaptureKeys, "capture-keys", false, "capture system keyboard shortcuts in the guest")
	flags.BoolVar(&opts.AutoResize, "auto-resize", false, "automatically reconfigure the guest display")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if opts.Autostart && opts.iso == "" {
		return opts, fmt.Errorf("-autostart requires -iso")
	}
	var output os.FileInfo
	if opts.serialLog != "" {
		var err error
		output, err = os.Stat(opts.serialLog)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return opts, err
		}
	}
	var inputs [2]os.FileInfo
	for i, path := range []string{opts.iso, opts.disk} {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return opts, err
		}
		if output != nil && os.SameFile(info, output) {
			return opts, fmt.Errorf("serial log would overwrite input: %s", path)
		}
		if !info.Mode().IsRegular() {
			return opts, fmt.Errorf("not a regular file: %s", path)
		}
		inputs[i] = info
	}
	if inputs[0] != nil && inputs[1] != nil && os.SameFile(inputs[0], inputs[1]) {
		return opts, fmt.Errorf("writable disk must not be the ISO image")
	}
	return opts, nil
}
