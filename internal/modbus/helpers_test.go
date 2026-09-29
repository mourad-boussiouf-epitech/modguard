package modbus

import "encoding/binary"

// adu builds a valid Modbus/TCP frame: an MBAP header with the right length, then the PDU.
func adu(txID uint16, unit uint8, pdu []byte) []byte {
	return append(header(txID, 0, uint16(len(pdu)+1), unit), pdu...)
}

// header builds an MBAP header from any field values, including invalid ones.
func header(txID, protocolID, length uint16, unit uint8) []byte {
	b := binary.BigEndian.AppendUint16(nil, txID)
	b = binary.BigEndian.AppendUint16(b, protocolID)
	b = binary.BigEndian.AppendUint16(b, length)
	return append(b, unit)
}

// pduOf is a function code followed by 16-bit big-endian fields.
func pduOf(fc FunctionCode, fields ...uint16) []byte {
	b := []byte{byte(fc)}
	for _, f := range fields {
		b = binary.BigEndian.AppendUint16(b, f)
	}
	return b
}

func readPDU(fc FunctionCode, addr, qty uint16) []byte {
	return pduOf(fc, addr, qty)
}

func writeSinglePDU(fc FunctionCode, addr, value uint16) []byte {
	return pduOf(fc, addr, value)
}

func writeMultiplePDU(fc FunctionCode, addr, qty uint16, byteCount uint8, data []byte) []byte {
	return append(append(pduOf(fc, addr, qty), byteCount), data...)
}

func registers(values ...uint16) []byte {
	var b []byte
	for _, v := range values {
		b = binary.BigEndian.AppendUint16(b, v)
	}
	return b
}
