package vz

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// PlatformConfiguration is an interface for a platform configuration.
type PlatformConfiguration interface {
	objc.NSObject

	platformConfiguration()
}

type basePlatformConfiguration struct{}

func (*basePlatformConfiguration) platformConfiguration() {}

// GenericPlatformConfiguration is the platform configuration for a generic Intel or ARM virtual machine.
type GenericPlatformConfiguration struct {
	*pointer

	*basePlatformConfiguration

	machineIdentifier *GenericMachineIdentifier
}

// MachineIdentifier returns the machine identifier.
func (m *GenericPlatformConfiguration) MachineIdentifier() *GenericMachineIdentifier {
	return m.machineIdentifier
}

// IsNestedVirtualizationSupported reports if nested virtualization is supported.
func IsNestedVirtualizationSupported() bool {
	if err := macOSAvailable(15); err != nil {
		return false
	}

	return vzbridge.VZGenericPlatformConfiguration_IsNestedVirtualizationSupported()
}

// SetNestedVirtualizationEnabled toggles nested virtualization.
func (m *GenericPlatformConfiguration) SetNestedVirtualizationEnabled(enable bool) error {
	if err := macOSAvailable(15); err != nil {
		return err
	}

	vzbridge.VZGenericPlatformConfiguration_SetNestedVirtualizationEnabled(m, bool(enable))
	return nil
}

var _ PlatformConfiguration = (*GenericPlatformConfiguration)(nil)

// NewGenericPlatformConfiguration creates a new generic platform configuration.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewGenericPlatformConfiguration(opts ...GenericPlatformConfigurationOption) (*GenericPlatformConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	platformConfig := &GenericPlatformConfiguration{
		pointer: vzbridge.VZGenericPlatformConfiguration_Init(),
	}
	for _, optFunc := range opts {
		if err := optFunc(platformConfig); err != nil {
			return nil, err
		}
	}
	return platformConfig, nil
}

// GenericMachineIdentifier is a struct that represents a unique identifier
// for a virtual machine.
type GenericMachineIdentifier struct {
	*pointer

	dataRepresentation []byte
}

// NewGenericMachineIdentifierWithDataPath initialize a new machine identifier described by the specified pathname.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewGenericMachineIdentifierWithDataPath(pathname string) (*GenericMachineIdentifier, error) {
	b, err := os.ReadFile(pathname)
	if err != nil {
		return nil, err
	}
	return NewGenericMachineIdentifierWithData(b)
}

// NewGenericMachineIdentifierWithData initialize a new machine identifier described by the specified data representation.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewGenericMachineIdentifierWithData(b []byte) (*GenericMachineIdentifier, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}

	ptr := vzbridge.NewVZGenericMachineIdentifierWithBytes(
		unsafe.Pointer(unsafe.SliceData(b)),
		int32(len(b)),
	)
	runtime.KeepAlive(b)
	if objc.Ptr(ptr) == nil {
		return nil, fmt.Errorf("invalid generic machine identifier data")
	}
	return newGenericMachineIdentifier(ptr), nil
}

// NewGenericMachineIdentifier initialize a new machine identifier is used by guests to uniquely
// identify the virtual hardware.
//
// Two virtual machines running concurrently should not use the same identifier.
//
// If the virtual machine is serialized to disk, the identifier can be preserved in a binary representation through
// DataRepresentation method.
// The identifier can then be recreated with NewGenericMachineIdentifierWithData function from the binary representation.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewGenericMachineIdentifier() (*GenericMachineIdentifier, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}
	return newGenericMachineIdentifier(vzbridge.VZGenericMachineIdentifier_New()), nil
}

func newGenericMachineIdentifier(ptr *objc.Pointer) *GenericMachineIdentifier {
	return &GenericMachineIdentifier{
		pointer:            ptr,
		dataRepresentation: nativeBytes(vzbridge.VZGenericMachineIdentifier_DataRepresentation(ptr)),
	}
}

// DataRepresentation opaque data representation of the machine identifier.
// This can be used to recreate the same machine identifier with NewGenericMachineIdentifierWithData function.
func (g *GenericMachineIdentifier) DataRepresentation() []byte { return g.dataRepresentation }

// GenericPlatformConfigurationOption is an optional function to create its configuration.
type GenericPlatformConfigurationOption func(*GenericPlatformConfiguration) error

// WithGenericMachineIdentifier is an option to create a new GenericPlatformConfiguration.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func WithGenericMachineIdentifier(m *GenericMachineIdentifier) GenericPlatformConfigurationOption {
	return func(mpc *GenericPlatformConfiguration) error {
		if err := macOSAvailable(13); err != nil {
			return err
		}
		mpc.machineIdentifier = m
		vzbridge.VZGenericPlatformConfiguration_SetMachineIdentifier(mpc, m)
		return nil
	}
}
