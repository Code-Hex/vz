package vz_test

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
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

func TestNewFileHandleSerialPortAttachment(t *testing.T) {
	for _, tt := range []struct {
		name  string
		read  *os.File
		write *os.File
	}{
		{"both", os.Stdin, os.Stderr},
		{"read only", os.Stdin, nil},
		{"write only", nil, os.Stderr},
		{"neither", nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			attachment, err := vz.NewFileHandleSerialPortAttachment(tt.read, tt.write)
			if errors.Is(err, vz.ErrUnsupportedOSVersion) {
				t.Skip(err)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer objc.Release(attachment)
			if objc.Ptr(attachment) == nil {
				t.Fatal("attachment wraps a NULL pointer")
			}
			read := vzbridge.VZFileHandleSerialPortAttachment_FileHandleForReading(attachment)
			write := vzbridge.VZFileHandleSerialPortAttachment_FileHandleForWriting(attachment)
			if read != nil {
				defer objc.Release(read)
			}
			if write != nil {
				defer objc.Release(write)
			}
			if (objc.Ptr(read) != nil) != (tt.read != nil) {
				t.Fatal("unexpected native reading handle")
			}
			if (objc.Ptr(write) != nil) != (tt.write != nil) {
				t.Fatal("unexpected native writing handle")
			}
		})
	}
}

func TestFileHandleSerialPortAttachmentOwnsDuplicate(t *testing.T) {
	for _, direction := range []string{"read", "write"} {
		t.Run(direction, func(t *testing.T) {
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			defer write.Close()
			var attachment *vz.FileHandleSerialPortAttachment
			var handle *objc.Pointer
			if direction == "read" {
				attachment, err = vz.NewFileHandleSerialPortAttachment(read, nil)
			} else {
				attachment, err = vz.NewFileHandleSerialPortAttachment(nil, write)
			}
			if errors.Is(err, vz.ErrUnsupportedOSVersion) {
				t.Skip(err)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer objc.Release(attachment)
			if direction == "read" {
				handle = vzbridge.VZFileHandleSerialPortAttachment_FileHandleForReading(attachment)
				if err := read.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				handle = vzbridge.VZFileHandleSerialPortAttachment_FileHandleForWriting(attachment)
				if err := write.Close(); err != nil {
					t.Fatal(err)
				}
			}
			defer objc.Release(handle)
			fd := int(pureobjc.Send[int32](pureobjc.ID(uintptr(objc.Ptr(handle))), pureobjc.RegisterName("fileDescriptor")))
			readFD, writeFD := int(read.Fd()), int(write.Fd())
			if direction == "read" {
				readFD = fd
			} else {
				writeFD = fd
			}
			if err := syscall.SetNonblock(readFD, true); err != nil {
				t.Fatal(err)
			}
			want := "serial data"
			if n, err := syscall.Write(writeFD, []byte(want)); err != nil || n != len(want) {
				t.Fatalf("write = (%d, %v), want (%d, nil)", n, err, len(want))
			}
			got := make([]byte, len(want))
			n, err := syscall.Read(readFD, got)
			if err != nil {
				t.Fatal(err)
			}
			if string(got[:n]) != want {
				t.Fatalf("read = %q, want %q", got[:n], want)
			}
		})
	}
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
		{name: "read only", read: file},
		{name: "write only", write: file},
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

	for _, files := range [][2]*os.File{
		{os.Stdin, os.Stderr}, {os.Stdin, nil}, {nil, os.Stderr}, {nil, nil},
	} {
		attachment, err := vz.NewFileHandleSerialPortAttachment(files[0], files[1])
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
