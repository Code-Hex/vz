//go:build darwin && arm64
// +build darwin,arm64

package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
# include "virtualization_arm64.h"
*/
import "C"
import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Code-Hex/vz/v2/internal/progress"
)

// MacHardwareModel describes a specific virtual Mac hardware model.
type MacHardwareModel struct {
	pointer

	supported          bool
	dataRepresentation []byte
}

// NewMacHardwareModelWithDataPath initialize a new hardware model described by the specified pathname.
func NewMacHardwareModelWithDataPath(pathname string) (*MacHardwareModel, error) {
	b, err := os.ReadFile(pathname)
	if err != nil {
		return nil, err
	}
	return NewMacHardwareModelWithData(b), nil
}

// NewMacHardwareModelWithData initialize a new hardware model described by the specified data representation.
func NewMacHardwareModelWithData(b []byte) *MacHardwareModel {
	ptr := C.newVZMacHardwareModelWithBytes(
		unsafe.Pointer(&b[0]),
		C.int(len(b)),
	)
	ret := newMacHardwareModel(ptr)
	runtime.SetFinalizer(ret, func(self *MacHardwareModel) {
		self.release()
	})
	return ret
}

func newMacHardwareModel(ptr unsafe.Pointer) *MacHardwareModel {
	ret := C.convertVZMacHardwareModel2Struct(ptr)
	dataRepresentation := ret.dataRepresentation
	bytePointer := (*byte)(unsafe.Pointer(dataRepresentation.ptr))
	return &MacHardwareModel{
		pointer: pointer{
			ptr: ptr,
		},
		supported: bool(ret.supported),
		// https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices
		dataRepresentation: unsafe.Slice(bytePointer, dataRepresentation.len),
	}
}

// Supported indicate whether this hardware model is supported by the host.
func (m *MacHardwareModel) Supported() bool { return m.supported }

// DataRepresentation opaque data representation of the hardware model.
// This can be used to recreate the same hardware model with NewMacHardwareModelWithData function.
func (m *MacHardwareModel) DataRepresentation() []byte { return m.dataRepresentation }

// MacMachineIdentifier an identifier to make a virtual machine unique.
type MacMachineIdentifier struct {
	pointer

	dataRepresentation []byte
}

// NewMacMachineIdentifierWithDataPath initialize a new machine identifier described by the specified pathname.
func NewMacMachineIdentifierWithDataPath(pathname string) (*MacMachineIdentifier, error) {
	b, err := os.ReadFile(pathname)
	if err != nil {
		return nil, err
	}
	return NewMacMachineIdentifierWithData(b), nil
}

// NewMacMachineIdentifierWithData initialize a new machine identifier described by the specified data representation.
func NewMacMachineIdentifierWithData(b []byte) *MacMachineIdentifier {
	ptr := C.newVZMacMachineIdentifierWithBytes(
		unsafe.Pointer(&b[0]),
		C.int(len(b)),
	)
	return newMacMachineIdentifier(ptr)
}

// NewMacMachineIdentifier initialize a new Mac machine identifier is used by macOS guests to uniquely
// identify the virtual hardware.
//
// Two virtual machines running concurrently should not use the same identifier.
//
// If the virtual machine is serialized to disk, the identifier can be preserved in a binary representation through
// DataRepresentation method.
// The identifier can then be recreated with NewMacMachineIdentifierWithData function from the binary representation.
func NewMacMachineIdentifier() *MacMachineIdentifier {
	return newMacMachineIdentifier(C.newVZMacMachineIdentifier())
}

func newMacMachineIdentifier(ptr unsafe.Pointer) *MacMachineIdentifier {
	dataRepresentation := C.getVZMacMachineIdentifierDataRepresentation(ptr)
	bytePointer := (*byte)(unsafe.Pointer(dataRepresentation.ptr))
	return &MacMachineIdentifier{
		pointer: pointer{
			ptr: ptr,
		},
		// https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices
		dataRepresentation: unsafe.Slice(bytePointer, dataRepresentation.len),
	}
}

// DataRepresentation opaque data representation of the machine identifier.
// This can be used to recreate the same machine identifier with NewMacMachineIdentifierWithData function.
func (m *MacMachineIdentifier) DataRepresentation() []byte { return m.dataRepresentation }

// MacAuxiliaryStorage is a struct that contains information the boot loader
// needs for booting macOS as a guest operating system.
type MacAuxiliaryStorage struct {
	pointer

	storagePath string
}

// NewMacAuxiliaryStorageOption is an option type to initialize a new Mac auxiliary storage
type NewMacAuxiliaryStorageOption func(*MacAuxiliaryStorage) error

// WithCreatingStorage is an option when initialize a new Mac auxiliary storage with data creation
// to you specified storage path.
func WithCreatingStorage(hardwareModel *MacHardwareModel) NewMacAuxiliaryStorageOption {
	return func(mas *MacAuxiliaryStorage) error {
		cpath := charWithGoString(mas.storagePath)
		defer cpath.Free()

		nserr := newNSErrorAsNil()
		nserrPtr := nserr.Ptr()
		mas.pointer = pointer{
			ptr: C.newVZMacAuxiliaryStorageWithCreating(
				cpath.CString(),
				hardwareModel.Ptr(),
				&nserrPtr,
			),
		}
		if err := newNSError(nserrPtr); err != nil {
			return err
		}
		return nil
	}
}

// NewMacAuxiliaryStorage creates a new MacAuxiliaryStorage is based Mac auxiliary storage data from the storagePath
// of an existing file by default.
func NewMacAuxiliaryStorage(storagePath string, opts ...NewMacAuxiliaryStorageOption) (*MacAuxiliaryStorage, error) {
	storage := &MacAuxiliaryStorage{storagePath: storagePath}
	for _, opt := range opts {
		if err := opt(storage); err != nil {
			return nil, err
		}
	}
	if storage.pointer.ptr == nil {
		cpath := charWithGoString(storagePath)
		defer cpath.Free()
		storage.pointer = pointer{
			ptr: C.newVZMacAuxiliaryStorage(cpath.CString()),
		}
	}
	return storage, nil
}

// MacOSRestoreImage is a struct that describes a version of macOS to install on to a virtual machine.
type MacOSRestoreImage struct {
	url                                     string
	buildVersion                            string
	operatingSystemVersion                  OperatingSystemVersion
	mostFeaturefulSupportedConfigurationPtr unsafe.Pointer
}

// URL returns URL of this restore image.
// the value of this property will be a file URL. (https://~)
// the value of this property will be a network URL referring to an installation media file. (file:///~)
func (m *MacOSRestoreImage) URL() string {
	return m.url
}

// BuildVersion returns the build version this restore image contains.
func (m *MacOSRestoreImage) BuildVersion() string {
	return m.buildVersion
}

// OperatingSystemVersion represents the operating system version this restore image contains.
type OperatingSystemVersion struct {
	MajorVersion int64
	MinorVersion int64
	PatchVersion int64
}

// String returns string for the build version this restore image contains.
func (osv OperatingSystemVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", osv.MajorVersion, osv.MinorVersion, osv.PatchVersion)
}

// OperatingSystemVersion returns the operating system version this restore image contains.
func (m *MacOSRestoreImage) OperatingSystemVersion() OperatingSystemVersion {
	return m.operatingSystemVersion
}

// MostFeaturefulSupportedConfiguration returns the configuration requirements for the most featureful
// configuration supported by the current host and by this restore image.
//
// A MacOSRestoreImage can contain installation media for multiple Mac hardware models (MacHardwareModel). Some of these
// hardware models may not be supported by the current host. This method can be used to determine the hardware model and
// configuration requirements that will provide the most complete feature set on the current host.
// If none of the hardware models are supported on the current host, this property is nil.
func (m *MacOSRestoreImage) MostFeaturefulSupportedConfiguration() *MacOSConfigurationRequirements {
	return newMacOSConfigurationRequirements(m.mostFeaturefulSupportedConfigurationPtr)
}

// MacOSConfigurationRequirements describes the parameter constraints required by a specific configuration of macOS.
//
//  When a VZMacOSRestoreImage is loaded, it can be inspected to determine the configurations supported by that restore image.
type MacOSConfigurationRequirements struct {
	minimumSupportedCPUCount   uint64
	minimumSupportedMemorySize uint64
	hardwareModelPtr           unsafe.Pointer
}

func newMacOSConfigurationRequirements(ptr unsafe.Pointer) *MacOSConfigurationRequirements {
	ret := C.convertVZMacOSConfigurationRequirements2Struct(ptr)
	return &MacOSConfigurationRequirements{
		minimumSupportedCPUCount:   uint64(ret.minimumSupportedCPUCount),
		minimumSupportedMemorySize: uint64(ret.minimumSupportedMemorySize),
		hardwareModelPtr:           ret.hardwareModel,
	}
}

// HardwareModel returns the hardware model for this configuration.
//
// The hardware model can be used to configure a new virtual machine that meets the requirements.
// Use VZMacPlatformConfiguration.hardwareModel to configure the Mac platform, and
// Use `WithCreatingStorage` functional option of the `NewMacAuxiliaryStorage` to create its auxiliary storage.
func (m *MacOSConfigurationRequirements) HardwareModel() *MacHardwareModel {
	return newMacHardwareModel(m.hardwareModelPtr)
}

// MinimumSupportedCPUCount returns the minimum supported number of CPUs for this configuration.
func (m *MacOSConfigurationRequirements) MinimumSupportedCPUCount() uint64 {
	return m.minimumSupportedCPUCount
}

// MinimumSupportedMemorySize returns the minimum supported memory size for this configuration.
func (m *MacOSConfigurationRequirements) MinimumSupportedMemorySize() uint64 {
	return m.minimumSupportedMemorySize
}

type macOSRestoreImageHandler func(restoreImage *MacOSRestoreImage, err error)

//export macOSRestoreImageCompletionHandler
func macOSRestoreImageCompletionHandler(cgoHandlerPtr, restoreImagePtr, errPtr unsafe.Pointer) {
	cgoHandler := *(*cgo.Handle)(cgoHandlerPtr)

	handler := cgoHandler.Value().(macOSRestoreImageHandler)
	defer cgoHandler.Delete()

	restoreImageStruct := (*C.VZMacOSRestoreImageStruct)(restoreImagePtr)

	restoreImage := &MacOSRestoreImage{
		url:          (*char)(restoreImageStruct.url).String(),
		buildVersion: (*char)(restoreImageStruct.buildVersion).String(),
		operatingSystemVersion: OperatingSystemVersion{
			MajorVersion: int64(restoreImageStruct.operatingSystemVersion.majorVersion),
			MinorVersion: int64(restoreImageStruct.operatingSystemVersion.minorVersion),
			PatchVersion: int64(restoreImageStruct.operatingSystemVersion.patchVersion),
		},
		mostFeaturefulSupportedConfigurationPtr: restoreImageStruct.mostFeaturefulSupportedConfiguration,
	}

	if err := newNSError(errPtr); err != nil {
		handler(restoreImage, err)
	} else {
		handler(restoreImage, nil)
	}
}

// downloadRestoreImage resumable downloads macOS restore image (ipsw) file.
func downloadRestoreImage(ctx context.Context, url string, destPath string) (*progress.Reader, error) {
	// open or create
	f, err := os.OpenFile(destPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		f.Close()
		return nil, err
	}

	req.Header.Add("User-Agent", "github.com/Code-Hex/vz")
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-", fileInfo.Size()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.Close()
		return nil, err
	}

	if 200 > resp.StatusCode || resp.StatusCode >= 300 {
		f.Close()
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected http status code: %d", resp.StatusCode)
	}

	reader := progress.NewReader(resp.Body, resp.ContentLength, fileInfo.Size())

	go func() {
		defer f.Close()
		defer resp.Body.Close()
		_, err := io.Copy(f, reader)
		reader.Finish(err)
	}()

	return reader, nil
}

// FetchLatestSupportedMacOSRestoreImage fetches the latest macOS restore image supported by this host from the network.
//
// After downloading the restore image, you can initialize a MacOSInstaller using LoadMacOSRestoreImageFromPath function
// with the local restore image file.
func FetchLatestSupportedMacOSRestoreImage(ctx context.Context, destPath string) (*progress.Reader, error) {
	waitCh := make(chan struct{})
	var (
		url      string
		fetchErr error
	)
	handler := macOSRestoreImageHandler(func(restoreImage *MacOSRestoreImage, err error) {
		url = restoreImage.URL()
		fetchErr = err
		defer close(waitCh)
	})
	cgoHandler := cgo.NewHandle(handler)
	C.fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(
		unsafe.Pointer(&cgoHandler),
	)
	<-waitCh
	if fetchErr != nil {
		return nil, fetchErr
	}
	progressReader, err := downloadRestoreImage(ctx, url, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to download from %q: %w", url, err)
	}
	return progressReader, nil
}

// LoadMacOSRestoreImageFromPath loads a macOS restore image from a filepath on the local file system.
//
// If the imagePath parameter doesn’t refer to a local file, the system raises an exception via Objective-C.
func LoadMacOSRestoreImageFromPath(imagePath string) (retImage *MacOSRestoreImage, retErr error) {
	waitCh := make(chan struct{})
	handler := macOSRestoreImageHandler(func(restoreImage *MacOSRestoreImage, err error) {
		retImage = restoreImage
		retErr = err
		close(waitCh)
	})
	cgoHandler := cgo.NewHandle(handler)

	cs := charWithGoString(imagePath)
	defer cs.Free()
	C.loadMacOSRestoreImageFile(cs.CString(), unsafe.Pointer(&cgoHandler))
	<-waitCh
	return
}

// MacOSInstaller is a struct you use to install macOS on the specified virtual machine.
type MacOSInstaller struct {
	pointer
	observerPointer pointer

	vm       *VirtualMachine
	progress atomic.Value
	doneCh   chan struct{}
	once     sync.Once
	err      error
}

// NewMacOSInstaller creates a new MacOSInstaller struct.
//
// A param vm is the virtual machine that the operating system will be installed onto.
// A param restoreImageIpsw is a file path indicating the macOS restore image to install.
func NewMacOSInstaller(vm *VirtualMachine, restoreImageIpsw string) *MacOSInstaller {
	cs := charWithGoString(restoreImageIpsw)
	defer cs.Free()
	ret := &MacOSInstaller{
		pointer: pointer{
			ptr: C.newVZMacOSInstaller(vm.Ptr(), vm.dispatchQueue, cs.CString()),
		},
		observerPointer: pointer{
			ptr: C.newProgressObserverVZMacOSInstaller(),
		},
		vm:     vm,
		doneCh: make(chan struct{}),
	}
	ret.setFractionCompleted(0)
	runtime.SetFinalizer(ret, func(self *MacOSInstaller) {
		self.observerPointer.release()
		self.release()
	})
	return ret
}

//export macOSInstallCompletionHandler
func macOSInstallCompletionHandler(cgoHandlerPtr, errPtr unsafe.Pointer) {
	cgoHandler := *(*cgo.Handle)(cgoHandlerPtr)

	handler := cgoHandler.Value().(func(error))
	defer cgoHandler.Delete()

	if err := newNSError(errPtr); err != nil {
		handler(err)
	} else {
		handler(nil)
	}
}

//export macOSInstallFractionCompletedHandler
func macOSInstallFractionCompletedHandler(cgoHandlerPtr unsafe.Pointer, completed C.double) {
	cgoHandler := *(*cgo.Handle)(cgoHandlerPtr)

	handler := cgoHandler.Value().(func(float64))
	handler(float64(completed))
}

// Install starts installing macOS.
//
// This method starts the installation process. The VM must be in a stopped state.
// During the installation operation, pausing or stopping the VM results in an undefined behavior.
func (m *MacOSInstaller) Install(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.once.Do(func() {
		completionHandler := cgo.NewHandle(func(err error) {
			m.err = err
			close(m.doneCh)
		})
		fractionCompletedHandler := cgo.NewHandle(func(v float64) {
			m.setFractionCompleted(v)
		})

		C.installByVZMacOSInstaller(
			m.Ptr(),
			m.vm.dispatchQueue,
			m.observerPointer.Ptr(),
			unsafe.Pointer(&completionHandler),
			unsafe.Pointer(&fractionCompletedHandler),
		)
	})

	select {
	case <-ctx.Done():
		C.cancelInstallVZMacOSInstaller(m.Ptr())
		return ctx.Err()
	case <-m.doneCh:
	}

	return m.err
}

func (m *MacOSInstaller) setFractionCompleted(completed float64) {
	m.progress.Store(completed)
}

// FractionCompleted returns the fraction of the overall work that the install process
// completes.
func (m *MacOSInstaller) FractionCompleted() float64 {
	return m.progress.Load().(float64)
}

// Done recieves a notification that indicates the install process is completed.
func (m *MacOSInstaller) Done() <-chan struct{} { return m.doneCh }
