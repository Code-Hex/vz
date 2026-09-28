package vz_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestNewFileHandleSerialPortAttachment guards against a regression where the
// constructor checked the error out-parameter pointer (always non-nil) instead
// of the duplicated file handle, causing it to return a non-nil wrapper around a
// NULL Objective-C object with a nil error. The serial console was silently lost.
func TestNewFileHandleSerialPortAttachment(t *testing.T) {
	attachment, err := vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stderr)
	if errors.Is(err, vz.ErrUnsupportedOSVersion) {
		t.Skipf("not supported on this macOS version: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if objc.Ptr(attachment) == nil {
		t.Fatal("attachment wraps a NULL pointer: constructor reported success but built nothing")
	}
}
