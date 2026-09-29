package vz_test

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/vzbridge"
)

func openFileDescriptorCount(t *testing.T) int {
	t.Helper()
	vzbridge.Drain()
	dir, err := os.Open("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	return len(names)
}

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
	objc.Release(attachment)
}

func TestNewFileHandleSerialPortAttachmentClosedFile(t *testing.T) {
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name  string
		read  *os.File
		write *os.File
	}{
		{name: "read", read: file, write: os.Stderr},
		{name: "write", read: os.Stdin, write: file},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := openFileDescriptorCount(t)
			attachment, err := vz.NewFileHandleSerialPortAttachment(tt.read, tt.write)
			if errors.Is(err, vz.ErrUnsupportedOSVersion) {
				t.Skipf("not supported on this macOS version: %v", err)
			}
			if attachment != nil {
				t.Fatal("expected no attachment for a closed file")
			}
			var nsErr *vz.NSError
			if !errors.As(err, &nsErr) || nsErr.Code != int(syscall.EBADF) {
				t.Fatalf("expected EBADF, got %v", err)
			}
			after := openFileDescriptorCount(t)
			if after != before {
				t.Fatalf("open file descriptors changed from %d to %d", before, after)
			}
		})
	}
}

func TestNewFileHandleSerialPortAttachmentClosesDuplicatedFiles(t *testing.T) {
	before := openFileDescriptorCount(t)

	for range 4 {
		attachment, err := vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stderr)
		if errors.Is(err, vz.ErrUnsupportedOSVersion) {
			t.Skipf("not supported on this macOS version: %v", err)
		}
		if err != nil {
			t.Fatal(err)
		}
		objc.Release(attachment)
	}

	after := openFileDescriptorCount(t)
	if after != before {
		t.Fatalf("open file descriptors changed from %d to %d", before, after)
	}
}
