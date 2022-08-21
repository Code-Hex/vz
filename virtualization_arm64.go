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
	"runtime/cgo"
	"unsafe"

	"github.com/Code-Hex/vz/v2/internal/progress"
)

type MacHardwareModel struct {
	pointer

	supported          bool
	dataRepresentation []byte
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

func (m *MacHardwareModel) Supported() bool            { return m.supported }
func (m *MacHardwareModel) DataRepresentation() []byte { return m.dataRepresentation }

type MacAuxiliaryStorage struct {
	pointer

	storagePath string
}

// NewMacAuxiliaryStorageOption is an option type to initialize a new Mac auxiliary storage
type NewMacAuxiliaryStorageOption func(*MacAuxiliaryStorage) error

// WithCreating is an option when initialize a new Mac auxiliary storage with data creation
// to you specified storage path.
func WithCreating(hardwareModel *MacHardwareModel) NewMacAuxiliaryStorageOption {
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

type MacOSRestoreImage struct {
	url                                     string
	buildVersion                            string
	operatingSystemVersion                  OperatingSystemVersion
	mostFeaturefulSupportedConfigurationPtr unsafe.Pointer
}

// URL returns URL of this restore image.
// the value of this property will be a file URL.
// the value of this property will be a network URL referring to an installation media file.
func (m *MacOSRestoreImage) URL() string {
	return m.url
}

// BuildVersion returns the build version this restore image contains.
func (m *MacOSRestoreImage) BuildVersion() string {
	return m.buildVersion
}

type OperatingSystemVersion struct {
	MajorVersion int64
	MinorVersion int64
	PatchVersion int64
}

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
// -[VZMacAuxiliaryStorage initCreatingStorageAtURL:hardwareModel:options:error:] to create its auxiliary storage.
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

type MacOSRestoreImageHandler func(restoreImage *MacOSRestoreImage, err error)

//export macOSRestoreImageCompletionHandler
func macOSRestoreImageCompletionHandler(cgoHandlerPtr, restoreImagePtr, errPtr unsafe.Pointer) {
	cgoHandler := *(*cgo.Handle)(cgoHandlerPtr)

	handler := cgoHandler.Value().(MacOSRestoreImageHandler)
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

func FetchLatestSupportedMacOSRestoreImage(ctx context.Context, destPath string) (*progress.Reader, error) {
	waitCh := make(chan struct{})
	var (
		url      string
		fetchErr error
	)
	handler := MacOSRestoreImageHandler(func(restoreImage *MacOSRestoreImage, err error) {
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

func LoadMacOSRestoreImageFromPath(imagePath string) (retImage *MacOSRestoreImage, retErr error) {
	waitCh := make(chan struct{})
	handler := MacOSRestoreImageHandler(func(restoreImage *MacOSRestoreImage, err error) {
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

type MacMachineIdentifier struct {
	pointer

	dataRepresentation []byte
}

type (
	MacMachineIdentifierOption func(*macMachineIdentifierOption)
	macMachineIdentifierOption struct {
		pointer unsafe.Pointer
	}
)

func WithBytes(b []byte) MacMachineIdentifierOption {
	return func(mmio *macMachineIdentifierOption) {
		mmio.pointer = C.newVZMacMachineIdentifierWithBytes(
			unsafe.Pointer(&b[0]),
			C.int(len(b)),
		)
	}
}

func NewMacMachineIdentifier(opts ...MacMachineIdentifierOption) *MacMachineIdentifier {
	opt := new(macMachineIdentifierOption)
	for _, optFunc := range opts {
		optFunc(opt)
	}
	if opt.pointer == nil {
		opt.pointer = C.newVZMacMachineIdentifier()
	}

	return newMacMachineIdentifier(opt.pointer)
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

func (m *MacMachineIdentifier) DataRepresentation() []byte { return m.dataRepresentation }
