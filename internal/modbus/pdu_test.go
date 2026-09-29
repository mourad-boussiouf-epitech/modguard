package modbus

import (
	"bytes"
	"errors"
	"testing"
)

const (
	diagReturnQueryData = 0x0000 // diagnostics sub-function: echo the data back
	meiReadDeviceID     = 0x0E
)

func TestParseRequest(t *testing.T) {
	coilBits := []byte{0b11001101, 0b00000001} // coils 0-7, then coils 8-9

	tests := []struct {
		name    string
		pdu     []byte
		want    Request
		wantErr error
	}{
		{
			name: "read coils",
			pdu:  readPDU(ReadCoils, 19, 19),
			want: Request{Function: ReadCoils, Address: 19, Quantity: 19},
		},
		{
			name: "read holding registers, max quantity",
			pdu:  readPDU(ReadHoldingRegisters, 0, 125),
			want: Request{Function: ReadHoldingRegisters, Quantity: 125},
		},
		{
			name: "read input registers up to the last address",
			pdu:  readPDU(ReadInputRegisters, 65535, 1),
			want: Request{Function: ReadInputRegisters, Address: 65535, Quantity: 1},
		},
		{
			name: "write single coil on",
			pdu:  writeSinglePDU(WriteSingleCoil, 172, CoilOn),
			want: Request{Function: WriteSingleCoil, Address: 172, Quantity: 1, Data: registers(CoilOn)},
		},
		{
			name: "write single register",
			pdu:  writeSinglePDU(WriteSingleRegister, 1, 3),
			want: Request{Function: WriteSingleRegister, Address: 1, Quantity: 1, Data: registers(3)},
		},
		{
			name: "write multiple coils",
			pdu:  writeMultiplePDU(WriteMultipleCoils, 19, 10, 2, coilBits),
			want: Request{Function: WriteMultipleCoils, Address: 19, Quantity: 10, Data: coilBits},
		},
		{
			name: "write multiple registers",
			pdu:  writeMultiplePDU(WriteMultipleRegisters, 1, 2, 4, registers(10, 258)),
			want: Request{Function: WriteMultipleRegisters, Address: 1, Quantity: 2, Data: registers(10, 258)},
		},
		{
			name: "diagnostics",
			pdu:  pduOf(Diagnostics, diagReturnQueryData, 0xA537),
			want: Request{Function: Diagnostics, Data: registers(diagReturnQueryData, 0xA537)},
		},
		{
			name: "read device identification",
			pdu:  []byte{byte(EncapsulatedInterface), meiReadDeviceID, 0x01, 0x00},
			want: Request{Function: EncapsulatedInterface, Data: []byte{meiReadDeviceID, 0x01, 0x00}},
		},

		{name: "empty", pdu: nil, wantErr: ErrEmptyPDU},
		{name: "unsupported function", pdu: pduOf(FunctionCode(0x07)), wantErr: IllegalFunction},
		{name: "exception bit set in a request", pdu: readPDU(ReadHoldingRegisters|exceptionFlag, 0, 1), wantErr: IllegalFunction},
		{name: "read too short", pdu: readPDU(ReadHoldingRegisters, 0, 1)[:4], wantErr: IllegalDataValue},
		{name: "read too long", pdu: append(readPDU(ReadHoldingRegisters, 0, 1), 0), wantErr: IllegalDataValue},
		{name: "read zero quantity", pdu: readPDU(ReadHoldingRegisters, 0, 0), wantErr: IllegalDataValue},
		{name: "read 126 registers", pdu: readPDU(ReadHoldingRegisters, 0, 126), wantErr: IllegalDataValue},
		{name: "read 2001 coils", pdu: readPDU(ReadCoils, 0, 2001), wantErr: IllegalDataValue},
		{name: "read past address 65535", pdu: readPDU(ReadInputRegisters, 65535, 2), wantErr: IllegalDataAddress},
		{name: "coil value neither on nor off", pdu: writeSinglePDU(WriteSingleCoil, 1, 0x1234), wantErr: IllegalDataValue},
		{name: "write single register too short", pdu: writeSinglePDU(WriteSingleRegister, 1, 3)[:4], wantErr: IllegalDataValue},
		{name: "write coils byte count mismatch", pdu: writeMultiplePDU(WriteMultipleCoils, 0, 10, 1, coilBits[:1]), wantErr: IllegalDataValue},
		{name: "write coils missing data", pdu: writeMultiplePDU(WriteMultipleCoils, 0, 10, 2, coilBits[:1]), wantErr: IllegalDataValue},
		{name: "write registers byte count mismatch", pdu: writeMultiplePDU(WriteMultipleRegisters, 0, 2, 2, registers(1)), wantErr: IllegalDataValue},
		{name: "write 124 registers", pdu: writeMultiplePDU(WriteMultipleRegisters, 0, 124, 248, make([]byte, 248)), wantErr: IllegalDataValue},
		{name: "write registers past address 65535", pdu: writeMultiplePDU(WriteMultipleRegisters, 65535, 2, 4, registers(0, 0)), wantErr: IllegalDataAddress},
		{name: "diagnostics without sub-function", pdu: []byte{byte(Diagnostics), 0x00}, wantErr: IllegalDataValue},
		{name: "encapsulated interface without MEI type", pdu: pduOf(EncapsulatedInterface), wantErr: IllegalDataValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRequest(tc.pdu)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got.Function != tc.want.Function || got.Address != tc.want.Address ||
				got.Quantity != tc.want.Quantity || !bytes.Equal(got.Data, tc.want.Data) {
				t.Errorf("request = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestIsWrite(t *testing.T) {
	writes := map[FunctionCode]bool{
		ReadCoils: false, ReadDiscreteInputs: false, ReadHoldingRegisters: false, ReadInputRegisters: false,
		WriteSingleCoil: true, WriteSingleRegister: true, WriteMultipleCoils: true, WriteMultipleRegisters: true,
		Diagnostics: false, EncapsulatedInterface: false,
	}
	for fc, want := range writes {
		if fc.IsWrite() != want {
			t.Errorf("0x%02X.IsWrite() = %v, want %v", uint8(fc), !want, want)
		}
	}
}
