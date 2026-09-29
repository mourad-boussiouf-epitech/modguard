package modbus

import (
	"bytes"
	"errors"
	"testing"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name    string
		pdu     []byte
		want    Request
		wantErr error
	}{
		{
			name: "read coils",
			pdu:  []byte{0x01, 0x00, 0x13, 0x00, 0x13},
			want: Request{Function: ReadCoils, Address: 0x13, Quantity: 0x13},
		},
		{
			name: "read holding registers, max quantity",
			pdu:  []byte{0x03, 0x00, 0x00, 0x00, 0x7D},
			want: Request{Function: ReadHoldingRegisters, Quantity: 125},
		},
		{
			name: "read input registers up to the last address",
			pdu:  []byte{0x04, 0xFF, 0xFF, 0x00, 0x01},
			want: Request{Function: ReadInputRegisters, Address: 0xFFFF, Quantity: 1},
		},
		{
			name: "write single coil on",
			pdu:  []byte{0x05, 0x00, 0xAC, 0xFF, 0x00},
			want: Request{Function: WriteSingleCoil, Address: 0xAC, Quantity: 1, Data: []byte{0xFF, 0x00}},
		},
		{
			name: "write single register",
			pdu:  []byte{0x06, 0x00, 0x01, 0x00, 0x03},
			want: Request{Function: WriteSingleRegister, Address: 1, Quantity: 1, Data: []byte{0x00, 0x03}},
		},
		{
			name: "write multiple coils",
			pdu:  []byte{0x0F, 0x00, 0x13, 0x00, 0x0A, 0x02, 0xCD, 0x01},
			want: Request{Function: WriteMultipleCoils, Address: 0x13, Quantity: 10, Data: []byte{0xCD, 0x01}},
		},
		{
			name: "write multiple registers",
			pdu:  []byte{0x10, 0x00, 0x01, 0x00, 0x02, 0x04, 0x00, 0x0A, 0x01, 0x02},
			want: Request{Function: WriteMultipleRegisters, Address: 1, Quantity: 2, Data: []byte{0x00, 0x0A, 0x01, 0x02}},
		},
		{
			name: "diagnostics",
			pdu:  []byte{0x08, 0x00, 0x00, 0xA5, 0x37},
			want: Request{Function: Diagnostics, Data: []byte{0x00, 0x00, 0xA5, 0x37}},
		},
		{
			name: "read device identification",
			pdu:  []byte{0x2B, 0x0E, 0x01, 0x00},
			want: Request{Function: EncapsulatedInterface, Data: []byte{0x0E, 0x01, 0x00}},
		},

		{name: "empty", pdu: nil, wantErr: ErrEmptyPDU},
		{name: "unknown function", pdu: []byte{0x07}, wantErr: IllegalFunction},
		{name: "exception bit set in a request", pdu: []byte{0x83, 0x00, 0x00, 0x00, 0x01}, wantErr: IllegalFunction},
		{name: "read too short", pdu: []byte{0x03, 0x00, 0x00, 0x00}, wantErr: IllegalDataValue},
		{name: "read too long", pdu: []byte{0x03, 0x00, 0x00, 0x00, 0x01, 0x00}, wantErr: IllegalDataValue},
		{name: "read zero quantity", pdu: []byte{0x03, 0x00, 0x00, 0x00, 0x00}, wantErr: IllegalDataValue},
		{name: "read 126 registers", pdu: []byte{0x03, 0x00, 0x00, 0x00, 0x7E}, wantErr: IllegalDataValue},
		{name: "read 2001 coils", pdu: []byte{0x01, 0x00, 0x00, 0x07, 0xD1}, wantErr: IllegalDataValue},
		{name: "read past address 65535", pdu: []byte{0x04, 0xFF, 0xFF, 0x00, 0x02}, wantErr: IllegalDataAddress},
		{name: "coil value not 0000 or FF00", pdu: []byte{0x05, 0x00, 0x01, 0x12, 0x34}, wantErr: IllegalDataValue},
		{name: "write single register too short", pdu: []byte{0x06, 0x00, 0x01, 0x00}, wantErr: IllegalDataValue},
		{name: "write coils byte count mismatch", pdu: []byte{0x0F, 0x00, 0x00, 0x00, 0x0A, 0x01, 0xCD}, wantErr: IllegalDataValue},
		{name: "write coils missing data", pdu: []byte{0x0F, 0x00, 0x00, 0x00, 0x0A, 0x02, 0xCD}, wantErr: IllegalDataValue},
		{name: "write registers byte count mismatch", pdu: []byte{0x10, 0x00, 0x00, 0x00, 0x02, 0x02, 0x00, 0x01}, wantErr: IllegalDataValue},
		{name: "write 124 registers", pdu: append([]byte{0x10, 0x00, 0x00, 0x00, 0x7C, 0xF8}, make([]byte, 248)...), wantErr: IllegalDataValue},
		{name: "write registers past address 65535", pdu: []byte{0x10, 0xFF, 0xFF, 0x00, 0x02, 0x04, 0, 0, 0, 0}, wantErr: IllegalDataAddress},
		{name: "diagnostics without sub-function", pdu: []byte{0x08, 0x00}, wantErr: IllegalDataValue},
		{name: "encapsulated interface without MEI type", pdu: []byte{0x2B}, wantErr: IllegalDataValue},
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
