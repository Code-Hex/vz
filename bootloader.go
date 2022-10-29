package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
# include "virtualization_13.h"
*/
import "C"
import (
	"fmt"
	"os"
	"runtime"
)

// BootLoader is the interface of boot loader definitions.
type BootLoader interface {
	NSObject

	bootLoader()
}

type baseBootLoader struct{}

func (*baseBootLoader) bootLoader() {}

var _ BootLoader = (*LinuxBootLoader)(nil)

// LinuxBootLoader Boot loader configuration for a Linux kernel.
// see: https://developer.apple.com/documentation/virtualization/vzlinuxbootloader?language=objc
type LinuxBootLoader struct {
	vmlinuzPath string
	initrdPath  string
	cmdLine     string
	pointer

	*baseBootLoader
}

func (b *LinuxBootLoader) String() string {
	return fmt.Sprintf(
		"vmlinuz: %q, initrd: %q, command-line: %q",
		b.vmlinuzPath,
		b.initrdPath,
		b.cmdLine,
	)
}

type LinuxBootLoaderOption func(b *LinuxBootLoader) error

// WithCommandLine sets the command-line parameters.
// see: https://www.kernel.org/doc/html/latest/admin-guide/kernel-parameters.html
func WithCommandLine(cmdLine string) LinuxBootLoaderOption {
	return func(b *LinuxBootLoader) error {
		b.cmdLine = cmdLine
		cs := charWithGoString(cmdLine)
		defer cs.Free()
		C.setCommandLineVZLinuxBootLoader(b.Ptr(), cs.CString())
		return nil
	}
}

// WithInitrd sets the optional initial RAM disk.
func WithInitrd(initrdPath string) LinuxBootLoaderOption {
	return func(b *LinuxBootLoader) error {
		if _, err := os.Stat(initrdPath); err != nil {
			return fmt.Errorf("invalid initial RAM disk path: %w", err)
		}
		b.initrdPath = initrdPath
		cs := charWithGoString(initrdPath)
		defer cs.Free()
		C.setInitialRamdiskURLVZLinuxBootLoader(b.Ptr(), cs.CString())
		return nil
	}
}

// NewLinuxBootLoader creates a LinuxBootLoader with the Linux kernel passed as Path.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewLinuxBootLoader(vmlinuz string, opts ...LinuxBootLoaderOption) (*LinuxBootLoader, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}
	if _, err := os.Stat(vmlinuz); err != nil {
		return nil, fmt.Errorf("invalid linux kernel path: %w", err)
	}

	vmlinuzPath := charWithGoString(vmlinuz)
	defer vmlinuzPath.Free()
	bootLoader := &LinuxBootLoader{
		vmlinuzPath: vmlinuz,
		pointer: pointer{
			ptr: C.newVZLinuxBootLoader(
				vmlinuzPath.CString(),
			),
		},
	}
	runtime.SetFinalizer(bootLoader, func(self *LinuxBootLoader) {
		self.Release()
	})
	for _, opt := range opts {
		if err := opt(bootLoader); err != nil {
			return nil, err
		}
	}
	return bootLoader, nil
}

var _ BootLoader = (*LinuxBootLoader)(nil)

// EFIBootLoader Boot loader configuration for booting guest operating systems expecting an EFI ROM.
// see: https://developer.apple.com/documentation/virtualization/vzefibootloader?language=objc
type EFIBootLoader struct {
	pointer

	*baseBootLoader

	variableStore *EFIVariableStore
}

// NewEFIBootLoaderOption is an option type to initialize a new EFIBootLoader.
type NewEFIBootLoaderOption func(b *EFIBootLoader)

// WithEFIVariableStore sets the optional EFI variable store.
func WithEFIVariableStore(variableStore *EFIVariableStore) NewEFIBootLoaderOption {
	return func(e *EFIBootLoader) {
		C.setVariableStoreVZEFIBootLoader(e.Ptr(), variableStore.Ptr())
		e.variableStore = variableStore
	}
}

// NewEFIBootLoader creates a new EFI boot loader.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewEFIBootLoader(opts ...NewEFIBootLoaderOption) (*EFIBootLoader, error) {
	if macosMajorVersionLessThan(13) {
		return nil, ErrUnsupportedOSVersion
	}
	bootLoader := &EFIBootLoader{
		pointer: pointer{
			ptr: C.newVZEFIBootLoader(),
		},
	}
	for _, optFunc := range opts {
		optFunc(bootLoader)
	}
	runtime.SetFinalizer(bootLoader, func(self *EFIBootLoader) {
		self.Release()
	})
	return bootLoader, nil
}

// VariableStore returns EFI variable store.
func (e *EFIBootLoader) VariableStore() *EFIVariableStore {
	return e.variableStore
}

// EFIVariableStore is EFI variable store.
// The EFI variable store contains NVRAM variables exposed by the EFI ROM.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore?language=objc
type EFIVariableStore struct {
	pointer

	path string
}

// NewEFIVariableStoreOption is an option type to initialize a new EFIVariableStore.
type NewEFIVariableStoreOption func(*EFIVariableStore) error

// WithCreatingEFIVariableStore is an option to initialized VZEFIVariableStore to a path on a file system.
// If the variable store already exists in path, it is overwritten.
func WithCreatingEFIVariableStore() NewEFIVariableStoreOption {
	return func(es *EFIVariableStore) error {
		cpath := charWithGoString(es.path)
		defer cpath.Free()

		nserr := newNSErrorAsNil()
		nserrPtr := nserr.Ptr()
		es.pointer = pointer{
			ptr: C.newCreatingVZEFIVariableStoreAtPath(
				cpath.CString(),
				&nserrPtr,
			),
		}
		if err := newNSError(nserrPtr); err != nil {
			return err
		}
		return nil
	}
}

// NewEFIVariableStore Initialize the variable store. If no options are specified,
// it initialises from the paths that exist.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewEFIVariableStore(path string, opts ...NewEFIVariableStoreOption) (*EFIVariableStore, error) {
	if macosMajorVersionLessThan(13) {
		return nil, ErrUnsupportedOSVersion
	}
	variableStore := &EFIVariableStore{path: path}
	for _, optFunc := range opts {
		if err := optFunc(variableStore); err != nil {
			return nil, err
		}
	}
	if variableStore.pointer.ptr == nil {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
		cpath := charWithGoString(path)
		defer cpath.Free()
		variableStore.pointer = pointer{
			ptr: C.newVZEFIVariableStorePath(cpath.CString()),
		}
	}
	runtime.SetFinalizer(variableStore, func(self *EFIVariableStore) {
		self.Release()
	})
	return variableStore, nil
}

// Path returns the path of the variable store on the local file system.
func (e *EFIVariableStore) Path() string { return e.path }
