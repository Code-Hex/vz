//go:build darwin && arm64
// +build darwin,arm64

package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization_arm64.h"
*/
import "C"
import (
	"runtime"
)

// MacPlatformConfiguration is the platform configuration for booting macOS on Apple silicon.
//
// When creating a VM, the hardwareModel and auxiliaryStorage depend on the restore image that you use to install macOS.
//
// To choose the hardware model, start from MacOSRestoreImage.MostFeaturefulSupportedConfiguration method to get a supported
// configuration, then use its MacOSConfigurationRequirements.HardwareModel method to get the hardware model.
//
// Use the hardware model to set up MacPlatformConfiguration and to initialize a new auxiliary storage with
// `WithCreatingStorage` functional option of the `NewMacAuxiliaryStorage`.
//
// When you save a VM to disk and load it again, you must restore the HardwareModel, MachineIdentifier and
// AuxiliaryStorage methods to their original values.
//
// If you create multiple VMs from the same configuration, each should have a unique auxiliaryStorage and machineIdentifier.
type MacPlatformConfiguration struct {
	pointer

	*basePlatformConfiguration

	hardwareModel     *MacHardwareModel
	machineIdentifier *MacMachineIdentifier
	auxiliaryStorage  *MacAuxiliaryStorage
}

var _ PlatformConfiguration = (*MacPlatformConfiguration)(nil)

// MacPlatformConfigurationOption is an optional function to create its configuration.
type MacPlatformConfigurationOption func(*MacPlatformConfiguration)

// WithHardwareModel is an option to create a new MacPlatformConfiguration.
func WithHardwareModel(m *MacHardwareModel) MacPlatformConfigurationOption {
	return func(mpc *MacPlatformConfiguration) {
		mpc.hardwareModel = m
		C.setHardwareModelVZMacPlatformConfiguration(mpc.Ptr(), m.Ptr())
	}
}

// WithMachineIdentifier is an option to create a new MacPlatformConfiguration.
func WithMachineIdentifier(m *MacMachineIdentifier) MacPlatformConfigurationOption {
	return func(mpc *MacPlatformConfiguration) {
		mpc.machineIdentifier = m
		C.setMachineIdentifierVZMacPlatformConfiguration(mpc.Ptr(), m.Ptr())
	}
}

// WithAuxiliaryStorage is an option to create a new MacPlatformConfiguration.
func WithAuxiliaryStorage(m *MacAuxiliaryStorage) MacPlatformConfigurationOption {
	return func(mpc *MacPlatformConfiguration) {
		mpc.auxiliaryStorage = m
		C.setAuxiliaryStorageVZMacPlatformConfiguration(mpc.Ptr(), m.Ptr())
	}
}

// NewMacPlatformConfiguration creates a new MacPlatformConfiguration. see also it's document.
func NewMacPlatformConfiguration(opts ...MacPlatformConfigurationOption) *MacPlatformConfiguration {
	platformConfig := &MacPlatformConfiguration{
		pointer: pointer{
			ptr: C.newVZMacPlatformConfiguration(),
		},
	}
	for _, optFunc := range opts {
		optFunc(platformConfig)
	}
	runtime.SetFinalizer(platformConfig, func(self *MacPlatformConfiguration) {
		self.Release()
	})
	return platformConfig
}

// HardwareModel returns the Mac hardware model.
func (m *MacPlatformConfiguration) HardwareModel() *MacHardwareModel { return m.hardwareModel }

// MachineIdentifier returns the Mac machine identifier.
func (m *MacPlatformConfiguration) MachineIdentifier() *MacMachineIdentifier {
	return m.machineIdentifier
}

// AuxiliaryStorage returns the Mac auxiliary storage.
func (m *MacPlatformConfiguration) AuxiliaryStorage() *MacAuxiliaryStorage { return m.auxiliaryStorage }
