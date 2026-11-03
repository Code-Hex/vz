//go:build darwin

package guiexample

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
)

func init() { runtime.LockOSThread() }

// Main parses command-line options and runs the selected GUI on the calling thread.
// The runner must close its views and guest before calling finish, which exits the process.
func Main(run func(Options, func(error))) {
	opts, err := parseOptions(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		finish(err)
		return
	}
	run(opts, finish)
}

func finish(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
