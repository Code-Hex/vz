package nbd

import (
	"encoding/binary"
	"io"
)

const (
	nbdMagic     = 0x4e42444d41474943
	optionMagic  = 0x49484156454f5054
	optionReply  = 0x3e889045565a9
	requestMagic = 0x25609513
	simpleReply  = 0x67446698

	fixedNewstyle = 1
	noZeroes      = 2

	optionExportName = 1
	optionAbort      = 2
	optionInfo       = 6
	optionGo         = 7

	replyACK         = 1
	replyInfo        = 3
	replyUnsupported = 1<<31 | 1
	replyInvalid     = 1<<31 | 3
	replyUnknown     = 1<<31 | 6

	infoExport    = 0
	infoBlockSize = 3

	flagHasFlags = 1
	flagReadOnly = 2
	flagFlush    = 4
	flagFUA      = 8

	commandRead       = 0
	commandWrite      = 1
	commandDisconnect = 2
	commandFlush      = 3
	commandFUA        = 1

	errorPermission = 1
	errorIO         = 5
	errorInvalid    = 22
	errorNoSpace    = 28

	maxRequest = 32 << 20
	maxOption  = 4096
)

func (s *Server) flags() uint16 {
	flags := uint16(flagHasFlags | flagFlush | flagFUA)
	if s.readOnly {
		flags |= flagReadOnly
	}
	return flags
}

func writeOption(w io.Writer, option, kind uint32, data []byte) error {
	header := struct {
		Magic  uint64
		Option uint32
		Kind   uint32
		Length uint32
	}{
		Magic:  optionReply,
		Option: option,
		Kind:   kind,
		Length: uint32(len(data)),
	}
	if err := binary.Write(w, binary.BigEndian, header); err != nil {
		return err
	}
	return writeFull(w, data)
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
