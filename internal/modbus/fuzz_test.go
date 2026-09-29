package modbus

import (
	"bytes"
	"errors"
	"testing"
)

// Seeds are the starting points the fuzzer mutates: one valid frame per
// request shape, plus a hostile header.
var seeds = []struct {
	name  string
	frame []byte
}{
	{"read 4 holding registers", adu(1, 1, readPDU(ReadHoldingRegisters, 0, 4))},
	{"switch coil 0 on", adu(2, 1, writeSinglePDU(WriteSingleCoil, 0, CoilOn))},
	{"write 2 registers", adu(3, 1, writeMultiplePDU(WriteMultipleRegisters, 0, 2, 4, registers(10, 258)))},
	{"write 10 coils", adu(4, 1, writeMultiplePDU(WriteMultipleCoils, 0, 10, 2, []byte{0b11001101, 0b00000001}))},
	{"read device identification", adu(5, 1, []byte{byte(EncapsulatedInterface), meiReadDeviceID, 0x01, 0x00})},
	{"header claiming 65535 bytes", header(6, 0, 65535, 1)},
}

func FuzzReadFrame(f *testing.F) {
	for _, s := range seeds {
		f.Add(s.frame)
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
	for _, s := range seeds {
		if len(s.frame) > HeaderLen {
			f.Add(s.frame[HeaderLen:])
		}
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
