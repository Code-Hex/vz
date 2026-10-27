package nbd

import (
	"encoding/binary"
	"errors"
	"io"
)

func (s *Server) negotiate(conn io.ReadWriter) error {
	greeting := struct {
		Magic   uint64
		Options uint64
		Flags   uint16
	}{
		Magic:   nbdMagic,
		Options: optionMagic,
		Flags:   fixedNewstyle | noZeroes,
	}
	if err := binary.Write(conn, binary.BigEndian, greeting); err != nil {
		return err
	}
	var flags uint32
	if err := binary.Read(conn, binary.BigEndian, &flags); err != nil {
		return err
	}
	if flags&fixedNewstyle == 0 || flags & ^uint32(fixedNewstyle|noZeroes) != 0 {
		return errors.New("nbd: invalid client flags")
	}
	for {
		var header struct {
			Magic  uint64
			Option uint32
			Length uint32
		}
		if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
			return err
		}
		if header.Magic != optionMagic || header.Length > maxOption {
			return errors.New("nbd: invalid option header")
		}
		// Consume each option body before replying, including unsupported
		// options, so the client can continue negotiation on this connection.
		data := make([]byte, header.Length)
		if _, err := io.ReadFull(conn, data); err != nil {
			return err
		}
		switch header.Option {
		case optionExportName:
			// EXPORT_NAME has no error reply. An unknown name ends negotiation;
			// INFO and GO below can reject a name without closing the connection.
			if len(data) != 0 {
				return errors.New("nbd: unknown export")
			}
			reply := make([]byte, 10)
			binary.BigEndian.PutUint64(reply, s.size)
			binary.BigEndian.PutUint16(reply[8:], s.flags())
			if flags&noZeroes == 0 {
				reply = append(reply, make([]byte, 124)...)
			}
			return writeFull(conn, reply)
		case optionAbort:
			if len(data) != 0 {
				if err := writeOption(conn, header.Option, replyInvalid, nil); err != nil {
					return err
				}
				continue
			}
			if err := writeOption(conn, header.Option, replyACK, nil); err != nil {
				return err
			}
			return io.EOF
		case optionInfo, optionGo:
			kind := validateInfo(data)
			if kind != 0 {
				if err := writeOption(conn, header.Option, kind, nil); err != nil {
					return err
				}
				continue
			}
			if err := s.writeInfo(conn, header.Option); err != nil {
				return err
			}
			if header.Option == optionGo {
				return nil
			}
		default:
			if err := writeOption(conn, header.Option, replyUnsupported, nil); err != nil {
				return err
			}
		}
	}
}

func validateInfo(data []byte) uint32 {
	if len(data) < 6 {
		return replyInvalid
	}
	nameLength := uint64(binary.BigEndian.Uint32(data))
	if nameLength > uint64(len(data)-6) {
		return replyInvalid
	}
	count := uint64(binary.BigEndian.Uint16(data[4+nameLength:]))
	if 6+nameLength+2*count != uint64(len(data)) {
		return replyInvalid
	}
	if nameLength != 0 {
		return replyUnknown
	}
	return 0
}

func (s *Server) writeInfo(w io.Writer, option uint32) error {
	var export [12]byte
	binary.BigEndian.PutUint16(export[:], infoExport)
	binary.BigEndian.PutUint64(export[2:], s.size)
	binary.BigEndian.PutUint16(export[10:], s.flags())
	if err := writeOption(w, option, replyInfo, export[:]); err != nil {
		return err
	}
	var geometry [14]byte
	binary.BigEndian.PutUint16(geometry[:], infoBlockSize)
	binary.BigEndian.PutUint32(geometry[2:], s.block)
	binary.BigEndian.PutUint32(geometry[6:], s.block)
	binary.BigEndian.PutUint32(geometry[10:], maxRequest)
	if err := writeOption(w, option, replyInfo, geometry[:]); err != nil {
		return err
	}
	return writeOption(w, option, replyACK, nil)
}
