package modbus

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	mb "github.com/simonvetter/modbus"
)

func TestExceptionResponse(t *testing.T) {
	tests := []struct {
		name string
		req  []byte
		code ExceptionCode
		want []byte
	}{
		{
			name: "blocked write single coil",
			req:  adu(42, 1, writeSinglePDU(WriteSingleCoil, 0, CoilOn)),
			code: IllegalFunction,
			want: adu(42, 1, []byte{0x85, 0x01}), // 0x05 | 0x80, illegal function
		},
		{
			name: "read outside allowed range",
			req:  adu(0xBEEF, 17, readPDU(ReadHoldingRegisters, 16, 2)),
			code: IllegalDataAddress,
			want: adu(0xBEEF, 17, []byte{0x83, 0x02}), // 0x03 | 0x80, illegal data address
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := ReadFrame(bytes.NewReader(tc.req))
			if err != nil {
				t.Fatal(err)
			}
			if got := ExceptionResponse(req, tc.code).Bytes(); !bytes.Equal(got, tc.want) {
				t.Errorf("got % X, want % X", got, tc.want)
			}
		})
	}
}

// A standard Modbus client must see our exception as a clean protocol error.
func TestExceptionResponseUnderstoodByClient(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			req, err := ReadFrame(conn)
			if err != nil {
				return
			}
			conn.Write(ExceptionResponse(req, IllegalFunction).Bytes())
		}
	}()

	c, err := mb.NewClient(&mb.ClientConfiguration{URL: "tcp://" + l.Addr().String(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Open(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.WriteCoil(0, true); !errors.Is(err, mb.ErrIllegalFunction) {
		t.Errorf("write coil: err = %v, want illegal function", err)
	}
	if _, err := c.ReadRegisters(0, 2, mb.HOLDING_REGISTER); !errors.Is(err, mb.ErrIllegalFunction) {
		t.Errorf("read registers on the same connection: err = %v, want illegal function", err)
	}
}
