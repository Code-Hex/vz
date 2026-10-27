package vz

import (
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// SpiceAgentPortAttachment is an attachment point that enables
// the Spice clipboard sharing capability.
//
// see: https://developer.apple.com/documentation/virtualization/vzspiceagentportattachment?language=objc
type SpiceAgentPortAttachment struct {
	*pointer

	*baseSerialPortAttachment
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
		pointer: vzbridge.VZSpiceAgentPortAttachment_Init(),
	}
	return spiceAgent, nil
}

// SetSharesClipboard enables or disables clipboard sharing through the Spice agent.
//
// See: https://developer.apple.com/documentation/virtualization/vzspiceagentportattachment/sharesclipboard?language=objc
func (s *SpiceAgentPortAttachment) SetSharesClipboard(enable bool) {
	vzbridge.VZSpiceAgentPortAttachment_SetSharesClipboard(s, enable)
}

// SharesClipboard reports whether clipboard sharing is enabled on the attachment.
//
// See: https://developer.apple.com/documentation/virtualization/vzspiceagentportattachment/sharesclipboard?language=objc
func (s *SpiceAgentPortAttachment) SharesClipboard() bool {
	return vzbridge.VZSpiceAgentPortAttachment_SharesClipboard(s)
}

// SpiceAgentPortAttachmentName returns the Spice agent port name.
func SpiceAgentPortAttachmentName() (string, error) {
	if err := macOSAvailable(13); err != nil {
		return "", err
	}
	return objcutil.String(vzbridge.VZSpiceAgentPortAttachment_SpiceAgentPortName()), nil
}
