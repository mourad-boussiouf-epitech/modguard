package plc

// Coils (FC 01, 05, 15)
const (
	CoilPump  uint16 = 0
	CoilValve uint16 = 1
	CoilAuto  uint16 = 2

	NumCoils = 3
)

// Discrete inputs (FC 02)
const (
	DIPumpRunning uint16 = 0
	DIValveOpen   uint16 = 1
	DILowAlarm    uint16 = 2
	DIHighAlarm   uint16 = 3
	DIOverflow    uint16 = 4

	NumDiscreteInputs = 5
)

// Input registers (FC 04)
const (
	IRLevel      uint16 = 0 // L
	IRLevelPct   uint16 = 1 // %
	IRInflow     uint16 = 2 // L/min
	IROutflow    uint16 = 3 // L/min
	IRPumpStarts uint16 = 4
	IRSpilled    uint16 = 5 // L
	IRCapacity   uint16 = 6 // L

	NumInputRegisters = 7
)

// Holding registers (FC 03, 06, 16), in litres
const (
	HRPumpOnLevel  uint16 = 0
	HRPumpOffLevel uint16 = 1
	HRLowAlarm     uint16 = 2
	HRHighAlarm    uint16 = 3

	NumHoldingRegisters = 4
)
