package vz_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	pureobjc "github.com/ebitengine/purego/objc"
	"golang.org/x/sys/unix"
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
		{
			"both",
			os.Stdin,
			os.Stderr,
		},
		{
			"read only",
			os.Stdin,
			nil,
		},
		{
			"write only",
			nil,
			os.Stderr,
		},
		{
			"neither",
			nil,
			nil,
		},
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
			alias := *attachment
			if err := attachment.Close(); err != nil {
				t.Fatalf("first close: %v", err)
			}
			err = alias.Close()
			if !errors.Is(err, vz.ErrSerialPortAttachmentClosed) {
				t.Fatalf("second close through alias: got %v, want ErrSerialPortAttachmentClosed", err)
			}
			if !errors.Is(err, os.ErrClosed) {
				t.Errorf("second close through alias: got %v, want os.ErrClosed", err)
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
			fd := serialFileHandleDescriptor(handle)
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
		{
			name:  "read",
			read:  file,
			write: os.Stderr,
		},
		{
			name:  "write",
			read:  os.Stdin,
			write: file,
		},
		{
			name: "read only",
			read: file,
		},
		{
			name:  "write only",
			write: file,
		},
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

func TestFileHandleSerialPortAttachmentCloseAfterStop(t *testing.T) {
	if vz.Available(12) {
		t.Skip("Stop requires macOS 12")
	}
	guestInput, hostInput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer guestInput.Close()
	defer hostInput.Close()
	hostOutput, guestOutput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer hostOutput.Close()
	defer guestOutput.Close()

	attachment, err := vz.NewFileHandleSerialPortAttachment(guestInput, guestOutput)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(attachment)
	readHandle := vzbridge.VZFileHandleSerialPortAttachment_FileHandleForReading(attachment)
	defer objc.Release(readHandle)
	writeHandle := vzbridge.VZFileHandleSerialPortAttachment_FileHandleForWriting(attachment)
	defer objc.Release(writeHandle)
	readFD := serialFileHandleDescriptor(readHandle)
	writeFD := serialFileHandleDescriptor(writeHandle)
	if err := guestOutput.Close(); err != nil {
		t.Fatal(err)
	}

	const guestInit = `console=hvc0 rdinit=/bin/busybox -- sh -c "` +
		`/bin/busybox mkdir -p /dev; /bin/busybox mount -t devtmpfs devtmpfs /dev; ` +
		`exec 3<> /dev/hvc0; /bin/busybox stty -echo <&3; printf 'serial ready\n' >&3; ` +
		`IFS= read -r boot <&3; printf 'serial tail %d\n' $boot >&3; exec /bin/busybox sleep 60"`
	loader, err := vz.NewLinuxBootLoader(
		"testdata/Image",
		vz.WithInitrd("testdata/initramfs.cpio.gz"),
		vz.WithCommandLine(guestInit),
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtualMachineConfiguration(loader, 1, 256*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	port, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{port})
	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.KeepAlive(config)
	defer objc.Release(vm)
	defer func() {
		if vm.CanStop() {
			if err := vm.Stop(); err != nil {
				t.Error(err)
			}
		}
	}()

	output := bufio.NewReader(hostOutput)
	for boot := 1; boot <= 2; boot++ {
		if err := vm.Start(); err != nil {
			t.Fatalf("start %d: %v", boot, err)
		}
		if err := hostOutput.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			t.Fatal(err)
		}
		for {
			line, err := output.ReadString('\n')
			if err != nil {
				t.Fatalf("read serial output from boot %d: %v", boot, err)
			}
			if strings.TrimSpace(line) == "serial ready" {
				break
			}
		}
		if boot == 1 {
			if err := attachment.Close(); !errors.Is(err, vz.ErrSerialPortAttachmentInUse) {
				t.Fatalf("close while VM is running: got %v, want ErrSerialPortAttachmentInUse", err)
			}
			if err := vm.Pause(); err != nil {
				t.Fatal(err)
			}
			if err := attachment.Close(); !errors.Is(err, vz.ErrSerialPortAttachmentInUse) {
				t.Fatalf("close while VM is paused: got %v, want ErrSerialPortAttachmentInUse", err)
			}
			if err := vm.Resume(); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := fmt.Fprintln(hostInput, boot); err != nil {
			t.Fatalf("write serial input to boot %d: %v", boot, err)
		}
		want := fmt.Sprintf("serial tail %d", boot)
		got, err := output.Peek(len(want))
		if err != nil || string(got) != want {
			t.Fatalf("read serial response from boot %d: got %q, %v, want %q", boot, got, err, want)
		}
		if err := vm.Stop(); err != nil {
			t.Fatalf("stop %d: %v", boot, err)
		}
		for _, fd := range []int{readFD, writeFD} {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != nil {
				t.Fatalf("serial descriptor %d after stop %d: %v", fd, boot, err)
			}
		}
	}

	if err := attachment.Close(); err != nil {
		t.Fatal(err)
	}
	if err := attachment.Close(); !errors.Is(err, vz.ErrSerialPortAttachmentClosed) {
		t.Fatalf("second close: got %v, want ErrSerialPortAttachmentClosed", err)
	}
	for _, fd := range []int{readFD, writeFD} {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
			t.Errorf("serial descriptor %d after close: want EBADF, got %v", fd, err)
		}
	}
	if _, err := unix.FcntlInt(guestInput.Fd(), unix.F_GETFD, 0); err != nil {
		t.Errorf("original guest input descriptor after close: %v", err)
	}
	if err := hostOutput.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(output)
	if err != nil {
		t.Fatalf("read serial output to EOF after close: %v", err)
	}
	if got := strings.TrimSpace(string(tail)); got != "serial tail 2" {
		t.Errorf("serial output before EOF = %q, want serial tail 2", got)
	}
	config.SetSerialPortsVirtualMachineConfiguration(nil)
	if !vm.CanStart() {
		t.Errorf("CanStart = false for a stopped VM with a closed serial attachment")
	}
	if err := vm.Start(); !errors.Is(err, vz.ErrSerialPortAttachmentClosed) {
		t.Errorf("start with closed serial attachment: got %v, want ErrSerialPortAttachmentClosed", err)
	}
	runtime.KeepAlive(vm)
}

func TestFileHandleSerialPortAttachmentCloseAfterAllVMsStop(t *testing.T) {
	if vz.Available(12) {
		t.Skip("Stop requires macOS 12")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	attachment, err := vz.NewFileHandleSerialPortAttachment(nil, null)
	if err != nil {
		t.Fatal(err)
	}
	defer objc.Release(attachment)
	loader, err := vz.NewLinuxBootLoader("testdata/Image", vz.WithInitrd("testdata/initramfs.cpio.gz"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := vz.NewVirtualMachineConfiguration(loader, 1, 256*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	port, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(attachment)
	if err != nil {
		t.Fatal(err)
	}
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{port})
	var machines [2]*vz.VirtualMachine
	for i := range machines {
		vm, err := vz.NewVirtualMachine(config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			defer objc.Release(vm)
			if vm.CanStop() {
				if err := vm.Stop(); err != nil {
					t.Error(err)
				}
			}
		})
		if err := vm.Start(); err != nil {
			t.Fatal(err)
		}
		machines[i] = vm
	}
	if err := machines[0].Stop(); err != nil {
		t.Fatal(err)
	}
	if err := attachment.Close(); !errors.Is(err, vz.ErrSerialPortAttachmentInUse) {
		t.Fatalf("close after first VM stopped: got %v, want ErrSerialPortAttachmentInUse", err)
	}
	if err := machines[1].Stop(); err != nil {
		t.Fatal(err)
	}
	if err := attachment.Close(); err != nil {
		t.Fatalf("close after both VMs stopped: %v", err)
	}
}

func serialFileHandleDescriptor(handle *objc.Pointer) int {
	fd := pureobjc.Send[int32](pureobjc.ID(uintptr(objc.Ptr(handle))), pureobjc.RegisterName("fileDescriptor"))
	runtime.KeepAlive(handle)
	return int(fd)
}
