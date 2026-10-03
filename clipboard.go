package vz

import "github.com/Code-Hex/vz/v4/internal/vzbridge"

// SpiceAgentPortAttachment is an attachment point that enables
// the Spice clipboard sharing capability.
//
// see: https://developer.apple.com/documentation/virtualization/vzspiceagentportattachment?language=objc
type SpiceAgentPortAttachment struct {
	*pointer

	*baseSerialPortAttachment

	enabledSharesClipboard bool
}

var _ SerialPortAttachment = (*SpiceAgentPortAttachment)(nil)

// NewSpiceAgentPortAttachment creates a new Spice agent port attachment.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewSpiceAgentPortAttachment() (*SpiceAgentPortAttachment, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}
	spiceAgent := &SpiceAgentPortAttachment{
		pointer:                vzbridge.VZSpiceAgentPortAttachment_Init(),
		enabledSharesClipboard: true,
	}
	return spiceAgent, nil
}

// SetSharesClipboard sets enable the Spice agent clipboard sharing capability.
func (s *SpiceAgentPortAttachment) SetSharesClipboard(enable bool) {
	vzbridge.VZSpiceAgentPortAttachment_SetSharesClipboard(s, bool(enable))
	s.enabledSharesClipboard = enable
}

// SharesClipboard returns enable the Spice agent clipboard sharing capability.
func (s *SpiceAgentPortAttachment) SharesClipboard() bool { return s.enabledSharesClipboard }

// SpiceAgentPortAttachmentName returns the Spice agent port name.
func SpiceAgentPortAttachmentName() (string, error) {
	if err := macOSAvailable(13); err != nil {
		return "", err
	}
	return nativeString(vzbridge.VZSpiceAgentPortAttachment_SpiceAgentPortName()), nil
}
