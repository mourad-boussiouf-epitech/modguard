package modbus

// A response echoes the request's function code with the high bit set to signal an exception.
const exceptionFlag = 0x80

// ExceptionResponse answers req with code. It keeps the transaction and unit
// IDs so the client can match the reply to its request.
func ExceptionResponse(req Frame, code ExceptionCode) Frame {
	var fc byte
	if len(req.PDU) > 0 {
		fc = req.PDU[0]
	}
	return Frame{
		Header: Header{TransactionID: req.TransactionID, Length: 3, UnitID: req.UnitID},
		PDU:    []byte{fc | exceptionFlag, byte(code)},
	}
}
