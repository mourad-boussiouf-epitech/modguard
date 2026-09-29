package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type FunctionCode uint8

const (
	ReadCoils              FunctionCode = 0x01
	ReadDiscreteInputs     FunctionCode = 0x02
	ReadHoldingRegisters   FunctionCode = 0x03
	ReadInputRegisters     FunctionCode = 0x04
	WriteSingleCoil        FunctionCode = 0x05
	WriteSingleRegister    FunctionCode = 0x06
	Diagnostics            FunctionCode = 0x08
	WriteMultipleCoils     FunctionCode = 0x0F
	WriteMultipleRegisters FunctionCode = 0x10
	EncapsulatedInterface  FunctionCode = 0x2B
)

func (fc FunctionCode) IsWrite() bool {
	switch fc {
	case WriteSingleCoil, WriteSingleRegister, WriteMultipleCoils, WriteMultipleRegisters:
		return true
	}
	return false
}

type ExceptionCode uint8

const (
	IllegalFunction    ExceptionCode = 0x01
	IllegalDataAddress ExceptionCode = 0x02
	IllegalDataValue   ExceptionCode = 0x03
)

func (e ExceptionCode) Error() string {
	names := map[ExceptionCode]string{
		IllegalFunction:    "illegal function",
		IllegalDataAddress: "illegal data address",
		IllegalDataValue:   "illegal data value",
	}
	name, ok := names[e]
	if !ok {
		name = "exception"
	}
	return fmt.Sprintf("modbus: %s (0x%02X)", name, uint8(e))
}

// Values a write single coil request may carry.
const (
	CoilOff uint16 = 0x0000
	CoilOn  uint16 = 0xFF00
)

// Addresses are 16 bits: 0 to 65535.
const addressSpace = 1 << 16

var ErrEmptyPDU = errors.New("modbus: empty pdu")

// Request is a decoded request PDU. Address and Quantity are zero for
// Diagnostics and EncapsulatedInterface, whose payload stays in Data.
type Request struct {
	Function FunctionCode
	Address  uint16
	Quantity uint16
	Data     []byte
}

// Quantity limits from the Modbus application protocol spec v1.1b3.
const (
	maxReadBits       = 2000
	maxReadRegisters  = 125
	maxWriteBits      = 1968
	maxWriteRegisters = 123
)

// ParseRequest validates a request PDU. Errors are ExceptionCode values,
// so they can be sent back to the client as is.
func ParseRequest(pdu []byte) (Request, error) {
	if len(pdu) == 0 {
		return Request{}, ErrEmptyPDU
	}
	req := Request{Function: FunctionCode(pdu[0])}
	body := pdu[1:]

	switch req.Function {
	case ReadCoils, ReadDiscreteInputs:
		return parseRead(req, body, maxReadBits)
	case ReadHoldingRegisters, ReadInputRegisters:
		return parseRead(req, body, maxReadRegisters)

	case WriteSingleCoil, WriteSingleRegister:
		if len(body) != 4 {
			return req, IllegalDataValue
		}
		req.Address = binary.BigEndian.Uint16(body[0:2])
		req.Quantity = 1
		req.Data = body[2:4]
		if req.Function == WriteSingleCoil {
			if v := binary.BigEndian.Uint16(req.Data); v != CoilOff && v != CoilOn {
				return req, IllegalDataValue
			}
		}
		return req, nil

	case WriteMultipleCoils:
		return parseWriteMultiple(req, body, maxWriteBits, func(q uint16) int { return (int(q) + 7) / 8 })
	case WriteMultipleRegisters:
		return parseWriteMultiple(req, body, maxWriteRegisters, func(q uint16) int { return 2 * int(q) })

	case Diagnostics:
		if len(body) < 2 {
			return req, IllegalDataValue
		}
		req.Data = body
		return req, nil
	case EncapsulatedInterface:
		if len(body) < 1 {
			return req, IllegalDataValue
		}
		req.Data = body
		return req, nil
	}
	return req, IllegalFunction
}

func parseRead(req Request, body []byte, maxQty uint16) (Request, error) {
	if len(body) != 4 {
		return req, IllegalDataValue
	}
	req.Address = binary.BigEndian.Uint16(body[0:2])
	req.Quantity = binary.BigEndian.Uint16(body[2:4])
	return req, checkRange(req, maxQty)
}

func parseWriteMultiple(req Request, body []byte, maxQty uint16, byteCount func(uint16) int) (Request, error) {
	if len(body) < 5 {
		return req, IllegalDataValue
	}
	req.Address = binary.BigEndian.Uint16(body[0:2])
	req.Quantity = binary.BigEndian.Uint16(body[2:4])
	if err := checkRange(req, maxQty); err != nil {
		return req, err
	}
	n := int(body[4])
	if n != byteCount(req.Quantity) || len(body) != 5+n {
		return req, IllegalDataValue
	}
	req.Data = body[5:]
	return req, nil
}

func checkRange(req Request, maxQty uint16) error {
	if req.Quantity == 0 || req.Quantity > maxQty {
		return IllegalDataValue
	}
	if int(req.Address)+int(req.Quantity) > addressSpace {
		return IllegalDataAddress
	}
	return nil
}
