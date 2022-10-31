package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
# include "virtualization_13.h"
*/
import "C"
import (
	"net"
	"os"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v2/internal/objc"
)

// BridgedNetwork defines a network interface that bridges a physical interface with a virtual machine.
//
// A bridged interface is shared between the virtual machine and the host system. Both host and
// virtual machine send and receive packets on the same physical interface but have distinct network layers.
//
// The BridgedNetwork can be used with a BridgedNetworkDeviceAttachment to set up a network device NetworkDeviceConfiguration.
// TODO(codehex): implement...
// see: https://developer.apple.com/documentation/virtualization/vzbridgednetworkinterface?language=objc
type BridgedNetwork interface {
	objc.NSObject

	// NetworkInterfaces returns the list of network interfaces available for bridging.
	NetworkInterfaces() []BridgedNetwork

	// Identifier returns the unique identifier for this interface.
	// The identifier is the BSD name associated with the interface (e.g. "en0").
	Identifier() string

	// LocalizedDisplayName returns a display name if available (e.g. "Ethernet").
	LocalizedDisplayName() string
}

// Network device attachment using network address translation (NAT) with outside networks.
//
// Using the NAT attachment type, the host serves as router and performs network address translation
// for accesses to outside networks.
// see: https://developer.apple.com/documentation/virtualization/vznatnetworkdeviceattachment?language=objc
type NATNetworkDeviceAttachment struct {
	*pointer

	*baseNetworkDeviceAttachment
}

var _ NetworkDeviceAttachment = (*NATNetworkDeviceAttachment)(nil)

// NewNATNetworkDeviceAttachment creates a new NATNetworkDeviceAttachment.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewNATNetworkDeviceAttachment() (*NATNetworkDeviceAttachment, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	attachment := &NATNetworkDeviceAttachment{
		pointer: objc.NewPointer(C.newVZNATNetworkDeviceAttachment()),
	}
	runtime.SetFinalizer(attachment, func(self *NATNetworkDeviceAttachment) {
		objc.Release(self)
	})
	return attachment, nil
}

// BridgedNetworkDeviceAttachment represents a physical interface on the host computer.
//
// Use this struct when configuring a network interface for your virtual machine.
// A bridged network device sends and receives packets on the same physical interface
// as the host computer, but does so using a different network layer.
//
// To use this attachment, your app must have the com.apple.vm.networking entitlement.
// If it doesn’t, the use of this attachment point results in an invalid VZVirtualMachineConfiguration object in objective-c.
//
// see: https://developer.apple.com/documentation/virtualization/vzbridgednetworkdeviceattachment?language=objc
type BridgedNetworkDeviceAttachment struct {
	*pointer

	*baseNetworkDeviceAttachment
}

var _ NetworkDeviceAttachment = (*BridgedNetworkDeviceAttachment)(nil)

// NewBridgedNetworkDeviceAttachment creates a new BridgedNetworkDeviceAttachment with networkInterface.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewBridgedNetworkDeviceAttachment(networkInterface BridgedNetwork) (*BridgedNetworkDeviceAttachment, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	attachment := &BridgedNetworkDeviceAttachment{
		pointer: objc.NewPointer(
			C.newVZBridgedNetworkDeviceAttachment(
				objc.Ptr(networkInterface),
			),
		),
	}
	runtime.SetFinalizer(attachment, func(self *BridgedNetworkDeviceAttachment) {
		objc.Release(self)
	})
	return attachment, nil
}

// FileHandleNetworkDeviceAttachment sending raw network packets over a file handle.
//
// The file handle attachment transmits the raw packets/frames between the virtual network interface and a file handle.
// The data transmitted through this attachment is at the level of the data link layer.
// see: https://developer.apple.com/documentation/virtualization/vzfilehandlenetworkdeviceattachment?language=objc
type FileHandleNetworkDeviceAttachment struct {
	*pointer

	*baseNetworkDeviceAttachment

	mtu int
}

var _ NetworkDeviceAttachment = (*FileHandleNetworkDeviceAttachment)(nil)

// NewFileHandleNetworkDeviceAttachment initialize the attachment with a file handle.
//
// file parameter is holding a connected datagram socket.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewFileHandleNetworkDeviceAttachment(file *os.File) (*FileHandleNetworkDeviceAttachment, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	attachment := &FileHandleNetworkDeviceAttachment{
		pointer: objc.NewPointer(
			C.newVZFileHandleNetworkDeviceAttachment(
				C.int(file.Fd()),
			),
		),
		mtu: 1500, // The default MTU is 1500.
	}
	runtime.SetFinalizer(attachment, func(self *FileHandleNetworkDeviceAttachment) {
		objc.Release(self)
	})
	return attachment, nil
}

// SetMaximumTransmissionUnit sets the maximum transmission unit (MTU) associated with this attachment.
//
// The maximum MTU allowed is 65535, and the minimum MTU allowed is 1500. An invalid MTU value will result in an invalid
// virtual machine configuration.
//
// The client side of the associated datagram socket must be properly configured with the appropriate values
// for SO_SNDBUF, and SO_RCVBUF. Set these using the setsockopt(_:_:_:_:_:) system call. The system expects
// the value of SO_RCVBUF to be at least double the value of SO_SNDBUF, and for optimal performance, the
// recommended value of SO_RCVBUF is four times the value of SO_SNDBUF.
//
// This is only supported on macOS 13 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func (f *FileHandleNetworkDeviceAttachment) SetMaximumTransmissionUnit(mtu int) error {
	if macosMajorVersionLessThan(13) {
		return ErrUnsupportedOSVersion
	}
	C.setMaximumTransmissionUnitVZFileHandleNetworkDeviceAttachment(
		objc.Ptr(f),
		C.NSInteger(mtu),
	)
	f.mtu = mtu
	return nil
}

// MaximumTransmissionUnit returns the maximum transmission unit (MTU) associated with this attachment.
// The default MTU is 1500.
func (f *FileHandleNetworkDeviceAttachment) MaximumTransmissionUnit() int {
	return f.mtu
}

// NetworkDeviceAttachment for a network device attachment.
// see: https://developer.apple.com/documentation/virtualization/vznetworkdeviceattachment?language=objc
type NetworkDeviceAttachment interface {
	objc.NSObject

	networkDeviceAttachment()
}

type baseNetworkDeviceAttachment struct{}

func (*baseNetworkDeviceAttachment) networkDeviceAttachment() {}

// VirtioNetworkDeviceConfiguration is configuration of a paravirtualized network device of type Virtio Network Device.
//
// The communication channel used on the host is defined through the attachment.
// It is set with the VZNetworkDeviceConfiguration.attachment property in objective-c.
//
// The configuration is only valid with valid MACAddress and attachment.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtionetworkdeviceconfiguration?language=objc
type VirtioNetworkDeviceConfiguration struct {
	*pointer
}

// NewVirtioNetworkDeviceConfiguration creates a new VirtioNetworkDeviceConfiguration with NetworkDeviceAttachment.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewVirtioNetworkDeviceConfiguration(attachment NetworkDeviceAttachment) (*VirtioNetworkDeviceConfiguration, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	config := newVirtioNetworkDeviceConfiguration(
		C.newVZVirtioNetworkDeviceConfiguration(
			objc.Ptr(attachment),
		),
	)
	runtime.SetFinalizer(config, func(self *VirtioNetworkDeviceConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

func newVirtioNetworkDeviceConfiguration(ptr unsafe.Pointer) *VirtioNetworkDeviceConfiguration {
	return &VirtioNetworkDeviceConfiguration{
		pointer: objc.NewPointer(ptr),
	}
}

func (v *VirtioNetworkDeviceConfiguration) SetMACAddress(macAddress *MACAddress) {
	C.setNetworkDevicesVZMACAddress(objc.Ptr(v), objc.Ptr(macAddress))
}

// MACAddress represents a media access control address (MAC address), the 48-bit ethernet address.
// see: https://developer.apple.com/documentation/virtualization/vzmacaddress?language=objc
type MACAddress struct {
	*pointer
}

// NewMACAddress creates a new MACAddress with net.HardwareAddr (MAC address).
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewMACAddress(macAddr net.HardwareAddr) (*MACAddress, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	macAddrChar := charWithGoString(macAddr.String())
	defer macAddrChar.Free()
	ma := &MACAddress{
		pointer: objc.NewPointer(
			C.newVZMACAddress(macAddrChar.CString()),
		),
	}
	runtime.SetFinalizer(ma, func(self *MACAddress) {
		objc.Release(self)
	})
	return ma, nil
}

// NewRandomLocallyAdministeredMACAddress creates a valid, random, unicast, locally administered address.
//
// This is only supported on macOS 11 and newer, ErrUnsupportedOSVersion will
// be returned on older versions.
func NewRandomLocallyAdministeredMACAddress() (*MACAddress, error) {
	if macosMajorVersionLessThan(11) {
		return nil, ErrUnsupportedOSVersion
	}

	ma := &MACAddress{
		pointer: objc.NewPointer(
			C.newRandomLocallyAdministeredVZMACAddress(),
		),
	}
	runtime.SetFinalizer(ma, func(self *MACAddress) {
		objc.Release(self)
	})
	return ma, nil
}

func (m *MACAddress) String() string {
	cstring := (*char)(C.getVZMACAddressString(objc.Ptr(m)))
	return cstring.String()
}

func (m *MACAddress) HardwareAddr() net.HardwareAddr {
	hw, _ := net.ParseMAC(m.String())
	return hw
}
