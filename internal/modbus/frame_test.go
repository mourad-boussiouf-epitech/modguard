package modbus

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadFrame(t *testing.T) {
	readHolding := readPDU(ReadHoldingRegisters, 0, 4)

	tests := []struct {
		name    string
		in      []byte
		want    Frame
		wantErr error
	}{
		{
			name: "read holding registers",
			in:   adu(1, 1, readHolding),
			want: Frame{Header: Header{TransactionID: 1, Length: 6, UnitID: 1}, PDU: readHolding},
		},
		{
			name: "function code only",
			in:   adu(0xABCD, 255, pduOf(ReadCoils)),
			want: Frame{Header: Header{TransactionID: 0xABCD, Length: 2, UnitID: 255}, PDU: pduOf(ReadCoils)},
		},
		{
			name: "largest allowed pdu",
			in:   adu(1, 1, make([]byte, MaxPDULen)),
			want: Frame{Header: Header{TransactionID: 1, Length: 254, UnitID: 1}, PDU: make([]byte, MaxPDULen)},
		},
		{name: "empty stream", in: nil, wantErr: io.EOF},
		{name: "truncated header", in: header(1, 0, 6, 1)[:3], wantErr: io.ErrUnexpectedEOF},
		{name: "truncated pdu", in: adu(1, 1, readHolding)[:9], wantErr: io.ErrUnexpectedEOF},
		{name: "header only, pdu missing", in: header(1, 0, 6, 1), wantErr: io.ErrUnexpectedEOF},
		{name: "non-zero protocol id", in: append(header(1, 1, 2, 1), byte(ReadCoils)), wantErr: ErrProtocolID},
		{name: "length 0", in: header(1, 0, 0, 1), wantErr: ErrLength},
		{name: "length 1 has no function code", in: header(1, 0, 1, 1), wantErr: ErrLength},
		{name: "length 255 is too long", in: header(1, 0, 255, 1), wantErr: ErrLength},
		{name: "length 65535 is rejected before allocating", in: header(1, 0, 65535, 1), wantErr: ErrLength},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadFrame(bytes.NewReader(tc.in))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got.Header != tc.want.Header || !bytes.Equal(got.PDU, tc.want.PDU) {
				t.Errorf("frame = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TCP may deliver a frame in pieces; ReadFrame must still assemble it.
func TestReadFrameSplitAcrossReads(t *testing.T) {
	in := adu(1, 1, readPDU(ReadHoldingRegisters, 0, 4))

	got, err := ReadFrame(&oneByteReader{data: in})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), in) {
		t.Errorf("got % X, want % X", got.Bytes(), in)
	}
}

func TestReadFramePipelined(t *testing.T) {
	first := adu(1, 1, readPDU(ReadHoldingRegisters, 0, 4))
	second := adu(2, 1, readPDU(ReadInputRegisters, 0, 7))
	r := bytes.NewReader(append(append([]byte{}, first...), second...))

	for i, want := range [][]byte{first, second} {
		f, err := ReadFrame(r)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if !bytes.Equal(f.Bytes(), want) {
			t.Errorf("frame %d = % X, want % X", i, f.Bytes(), want)
		}
	}
	if _, err := ReadFrame(r); !errors.Is(err, io.EOF) {
		t.Errorf("after last frame: err = %v, want EOF", err)
	}
}

// Pins the wire format byte by byte, independently of the test helpers.
func TestBytes(t *testing.T) {
	f := Frame{
		Header: Header{TransactionID: 0x1234, UnitID: 0x11},
		PDU:    writeSinglePDU(WriteSingleRegister, 1, 3),
	}
	want := []byte{
		0x12, 0x34, // transaction id
		0x00, 0x00, // protocol id
		0x00, 0x06, // length: unit id + 5 pdu bytes
		0x11,       // unit id
		0x06,       // function code: write single register
		0x00, 0x01, // address 1
		0x00, 0x03, // value 3
	}
	if got := f.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("Bytes() = % X, want % X", got, want)
	}
}

type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}
