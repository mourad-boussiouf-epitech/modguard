// Package modbus decodes and encodes Modbus/TCP frames.
package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	HeaderLen = 7
	MaxPDULen = 253

	// Length counts the unit ID plus the PDU, which holds at least a function code.
	minLength = 2
	maxLength = 1 + MaxPDULen
)

var (
	ErrProtocolID = errors.New("modbus: protocol id is not 0")
	ErrLength     = errors.New("modbus: invalid length field")
)

type Header struct {
	TransactionID uint16
	ProtocolID    uint16
	Length        uint16
	UnitID        uint8
}

type Frame struct {
	Header
	PDU []byte
}

func ParseHeader(b []byte) (Header, error) {
	if len(b) < HeaderLen {
		return Header{}, fmt.Errorf("modbus: header needs %d bytes, got %d", HeaderLen, len(b))
	}
	h := Header{
		TransactionID: binary.BigEndian.Uint16(b[0:2]),
		ProtocolID:    binary.BigEndian.Uint16(b[2:4]),
		Length:        binary.BigEndian.Uint16(b[4:6]),
		UnitID:        b[6],
	}
	if h.ProtocolID != 0 {
		return h, ErrProtocolID
	}
	if h.Length < minLength || h.Length > maxLength {
		return h, fmt.Errorf("%w: %d not in [%d, %d]", ErrLength, h.Length, minLength, maxLength)
	}
	return h, nil
}

// ReadFrame reads exactly one frame. The length field is checked before
// allocating, so a hostile header can't make us allocate more than 253 bytes.
func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [HeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	h, err := ParseHeader(hdr[:])
	if err != nil {
		return Frame{}, err
	}
	pdu := make([]byte, h.Length-1)
	if _, err := io.ReadFull(r, pdu); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return Frame{}, err
	}
	return Frame{Header: h, PDU: pdu}, nil
}

// Bytes encodes the frame, computing the length field from the PDU.
func (f Frame) Bytes() []byte {
	b := make([]byte, HeaderLen, HeaderLen+len(f.PDU))
	binary.BigEndian.PutUint16(b[0:2], f.TransactionID)
	binary.BigEndian.PutUint16(b[2:4], f.ProtocolID)
	binary.BigEndian.PutUint16(b[4:6], uint16(len(f.PDU)+1))
	b[6] = f.UnitID
	return append(b, f.PDU...)
}
