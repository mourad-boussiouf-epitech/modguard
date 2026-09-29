package modbus

import (
	"bytes"
	"errors"
	"testing"
)

var seedFrames = [][]byte{
	{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x04},
	{0x00, 0x02, 0x00, 0x00, 0x00, 0x06, 0x01, 0x05, 0x00, 0x00, 0xFF, 0x00},
	{0x00, 0x03, 0x00, 0x00, 0x00, 0x0B, 0x01, 0x10, 0x00, 0x00, 0x00, 0x02, 0x04, 0x00, 0x0A, 0x01, 0x02},
	{0x00, 0x04, 0x00, 0x00, 0x00, 0x09, 0x01, 0x0F, 0x00, 0x00, 0x00, 0x0A, 0x02, 0xCD, 0x01},
	{0x00, 0x05, 0x00, 0x00, 0x00, 0x05, 0x01, 0x2B, 0x0E, 0x01, 0x00},
	{0x00, 0x06, 0x00, 0x00, 0xFF, 0xFF, 0x01},
}

func FuzzReadFrame(f *testing.F) {
	for _, s := range seedFrames {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		fr, err := ReadFrame(bytes.NewReader(data))
		if err != nil {
			return
		}
		if fr.ProtocolID != 0 {
			t.Fatalf("accepted protocol id %d", fr.ProtocolID)
		}
		if len(fr.PDU) < 1 || len(fr.PDU) > MaxPDULen || len(fr.PDU) != int(fr.Length)-1 {
			t.Fatalf("pdu length %d with length field %d", len(fr.PDU), fr.Length)
		}
		n := HeaderLen + len(fr.PDU)
		if got := fr.Bytes(); !bytes.Equal(got, data[:n]) {
			t.Fatalf("round trip: got % X, want % X", got, data[:n])
		}
	})
}

func FuzzParseRequest(f *testing.F) {
	for _, s := range seedFrames {
		f.Add(s[HeaderLen:])
	}
	f.Fuzz(func(t *testing.T, pdu []byte) {
		req, err := ParseRequest(pdu)
		if err != nil {
			var exc ExceptionCode
			if !errors.As(err, &exc) && !errors.Is(err, ErrEmptyPDU) {
				t.Fatalf("unexpected error type %T: %v", err, err)
			}
			return
		}

		limits := map[FunctionCode]uint16{
			ReadCoils: maxReadBits, ReadDiscreteInputs: maxReadBits,
			ReadHoldingRegisters: maxReadRegisters, ReadInputRegisters: maxReadRegisters,
			WriteSingleCoil: 1, WriteSingleRegister: 1,
			WriteMultipleCoils: maxWriteBits, WriteMultipleRegisters: maxWriteRegisters,
		}
		maxQty, addressed := limits[req.Function]
		if !addressed {
			return
		}
		if req.Quantity == 0 || req.Quantity > maxQty {
			t.Fatalf("fc 0x%02X accepted quantity %d", uint8(req.Function), req.Quantity)
		}
		if int(req.Address)+int(req.Quantity) > 0x10000 {
			t.Fatalf("fc 0x%02X accepted range %d+%d past 65535", uint8(req.Function), req.Address, req.Quantity)
		}
		switch req.Function {
		case WriteMultipleCoils:
			if len(req.Data) != (int(req.Quantity)+7)/8 {
				t.Fatalf("%d coils with %d data bytes", req.Quantity, len(req.Data))
			}
		case WriteMultipleRegisters:
			if len(req.Data) != 2*int(req.Quantity) {
				t.Fatalf("%d registers with %d data bytes", req.Quantity, len(req.Data))
			}
		}
	})
}
