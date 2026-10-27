package nbd

import (
	"encoding/binary"
	"errors"
	"io"
	"syscall"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

type request struct {
	Magic  uint32
	Flags  uint16
	Kind   uint16
	Handle uint64
	Offset uint64
	Length uint32
}

func (s *Server) transmit(conn io.ReadWriter) error {
	for {
		var req request
		if err := binary.Read(conn, binary.BigEndian, &req); err != nil {
			return err
		}
		if req.Magic != requestMagic || req.Length > maxRequest {
			return errors.New("nbd: invalid request header")
		}
		if req.Kind == commandDisconnect {
			return nil
		}
		var data []byte
		if req.Kind == commandWrite {
			// Even a rejected WRITE has a payload. Consume it before replying
			// so the next request starts at its header, not in these bytes.
			data = make([]byte, req.Length)
			if _, err := io.ReadFull(conn, data); err != nil {
				return err
			}
		}
		code := s.validateRequest(req)
		if code == 0 {
			data, code = s.execute(req, data)
		}
		reply := struct {
			Magic  uint32
			Error  uint32
			Handle uint64
		}{
			Magic:  simpleReply,
			Error:  code,
			Handle: req.Handle,
		}
		if err := binary.Write(conn, binary.BigEndian, reply); err != nil {
			return err
		}
		if req.Kind == commandRead && code == 0 {
			if err := writeFull(conn, data); err != nil {
				return err
			}
		}
	}
}

func (s *Server) validateRequest(req request) uint32 {
	if req.Kind == commandFlush {
		if req.Flags & ^uint16(commandFUA) != 0 || req.Offset != 0 || req.Length != 0 {
			return errorInvalid
		}
		return 0
	}
	if req.Kind != commandRead && req.Kind != commandWrite {
		return errorInvalid
	}
	if req.Flags & ^uint16(commandFUA) != 0 || req.Length == 0 {
		return errorInvalid
	}
	if req.Offset > s.size || uint64(req.Length) > s.size-req.Offset {
		return errorInvalid
	}
	if req.Offset%uint64(s.block) != 0 || req.Length%s.block != 0 {
		return errorInvalid
	}
	if req.Kind == commandWrite && s.readOnly {
		return errorPermission
	}
	return 0
}

func (s *Server) execute(req request, data []byte) ([]byte, uint32) {
	var err error
	switch req.Kind {
	case commandRead:
		data = make([]byte, req.Length)
		_, err = s.image.ReadAt(data, int64(req.Offset))
	case commandWrite:
		_, err = s.image.WriteAt(data, int64(req.Offset))
		if err == nil {
			// A client may expose this export as a write-through disk and omit
			// FUA. Sync every successful write before acknowledging it.
			err = s.image.Sync()
		}
	case commandFlush:
		err = s.image.Sync()
	}
	if err == nil {
		return data, 0
	}
	if errors.Is(err, syscall.ENOSPC) {
		return nil, errorNoSpace
	}
	if req.Kind == commandWrite && errors.Is(err, diskimagekit.ErrReadOnly) {
		return nil, errorPermission
	}
	return nil, errorIO
}
