package modbus

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadFrame(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    Frame
		wantErr error
	}{
		{
			name: "read holding registers",
			in:   []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x04},
			want: Frame{
				Header: Header{TransactionID: 1, Length: 6, UnitID: 1},
				PDU:    []byte{0x03, 0x00, 0x00, 0x00, 0x04},
			},
		},
		{
			name: "function code only",
			in:   []byte{0xAB, 0xCD, 0x00, 0x00, 0x00, 0x02, 0xFF, 0x07},
			want: Frame{
				Header: Header{TransactionID: 0xABCD, Length: 2, UnitID: 0xFF},
				PDU:    []byte{0x07},
			},
		},
		{
			name: "largest allowed pdu",
			in:   append([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0xFE, 0x01}, make([]byte, MaxPDULen)...),
			want: Frame{
				Header: Header{TransactionID: 1, Length: 254, UnitID: 1},
				PDU:    make([]byte, MaxPDULen),
			},
		},
		{
			name:    "empty stream",
			in:      nil,
			wantErr: io.EOF,
		},
		{
			name:    "truncated header",
			in:      []byte{0x00, 0x01, 0x00},
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "truncated pdu",
			in:      []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00},
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "header only, pdu missing",
			in:      []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01},
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "non-zero protocol id",
			in:      []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x02, 0x01, 0x03},
			wantErr: ErrProtocolID,
		},
		{
			name:    "length 0",
			in:      []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x01},
			wantErr: ErrLength,
		},
		{
			name:    "length 1 has no function code",
			in:      []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x01},
			wantErr: ErrLength,
		},
		{
			name:    "length 255 is too long",
			in:      []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0xFF, 0x01},
			wantErr: ErrLength,
		},
		{
			name:    "length 65535 is rejected before allocating",
			in:      []byte{0x00, 0x01, 0x00, 0x00, 0xFF, 0xFF, 0x01},
			wantErr: ErrLength,
		},
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
	in := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x04}
	r := &oneByteReader{data: in}

	got, err := ReadFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), in) {
		t.Errorf("got % X, want % X", got.Bytes(), in)
	}
}

func TestReadFramePipelined(t *testing.T) {
	first := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x04}
	second := []byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x06, 0x01, 0x04, 0x00, 0x00, 0x00, 0x07}
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

func TestBytes(t *testing.T) {
	f := Frame{
		Header: Header{TransactionID: 0x1234, UnitID: 0x11},
		PDU:    []byte{0x06, 0x00, 0x01, 0x00, 0x03},
	}
	want := []byte{0x12, 0x34, 0x00, 0x00, 0x00, 0x06, 0x11, 0x06, 0x00, 0x01, 0x00, 0x03}
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
