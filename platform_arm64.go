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

type MacPlatformConfiguration struct {
	pointer

	*basePlatformConfiguration

	hardwareModel     *MacHardwareModel
	machineIdentifier *MacMachineIdentifier
	auxiliaryStorage  *MacAuxiliaryStorage
}

var _ PlatformConfiguration = (*MacPlatformConfiguration)(nil)

type MacPlatformConfigurationOption func(*MacPlatformConfiguration)

func WithHardwareModel(m *MacHardwareModel) MacPlatformConfigurationOption {
	return func(mpc *MacPlatformConfiguration) {
		mpc.hardwareModel = m
		C.setHardwareModelVZMacPlatformConfiguration(mpc.Ptr(), m.Ptr())
	}
}

func WithMachineIdentifier(m *MacMachineIdentifier) MacPlatformConfigurationOption {
	return func(mpc *MacPlatformConfiguration) {
		mpc.machineIdentifier = m
		C.setMachineIdentifierVZMacPlatformConfiguration(mpc.Ptr(), m.Ptr())
	}
}

func WithAuxiliaryStorage(m *MacAuxiliaryStorage) MacPlatformConfigurationOption {
	return func(mpc *MacPlatformConfiguration) {
		mpc.auxiliaryStorage = m
		C.setAuxiliaryStorageVZMacPlatformConfiguration(mpc.Ptr(), m.Ptr())
	}
}

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

func (m *MacPlatformConfiguration) HardwareModel() *MacHardwareModel { return m.hardwareModel }

func (m *MacPlatformConfiguration) MachineIdentifier() *MacMachineIdentifier {
	return m.machineIdentifier
}
func (m *MacPlatformConfiguration) AuxiliaryStorage() *MacAuxiliaryStorage { return m.auxiliaryStorage }
