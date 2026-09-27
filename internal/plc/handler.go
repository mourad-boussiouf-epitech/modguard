package plc

import (
	"math"
	"slices"

	"github.com/simonvetter/modbus"
)

// The modbus server compares errors with ==, so return them unwrapped.
var _ modbus.RequestHandler = (*PLC)(nil)

func (p *PLC) HandleCoils(req *modbus.CoilsRequest) ([]bool, error) {
	log := p.log.With("client", req.ClientAddr, "unit", req.UnitId, "table", "coils",
		"addr", req.Addr, "qty", req.Quantity)
	if !inRange(req.Addr, req.Quantity, NumCoils) {
		log.Warn("rejected: illegal data address", "write", req.IsWrite)
		return nil, modbus.ErrIllegalDataAddress
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	coils := []bool{CoilPump: p.pump, CoilValve: p.valve, CoilAuto: p.auto}
	if !req.IsWrite {
		log.Debug("read")
		return coils[req.Addr : req.Addr+req.Quantity], nil
	}

	next := slices.Clone(coils)
	copy(next[req.Addr:], req.Args)
	if next[CoilAuto] && next[CoilPump] != p.pump {
		log.Warn("rejected: pump is under automatic control, turn auto mode off first",
			"values", req.Args)
		return nil, modbus.ErrIllegalDataValue
	}

	p.valve = next[CoilValve]
	p.auto = next[CoilAuto]
	p.setPump(next[CoilPump])
	log.Info("write", "values", req.Args)
	return nil, nil
}

func (p *PLC) HandleDiscreteInputs(req *modbus.DiscreteInputsRequest) ([]bool, error) {
	log := p.log.With("client", req.ClientAddr, "unit", req.UnitId, "table", "discrete_inputs",
		"addr", req.Addr, "qty", req.Quantity)
	if !inRange(req.Addr, req.Quantity, NumDiscreteInputs) {
		log.Warn("rejected: illegal data address")
		return nil, modbus.ErrIllegalDataAddress
	}

	s := p.State()
	inputs := []bool{
		DIPumpRunning: s.Pump,
		DIValveOpen:   s.Valve,
		DILowAlarm:    s.LowAlarm,
		DIHighAlarm:   s.HighAlarm,
		DIOverflow:    s.Overflow,
	}
	log.Debug("read")
	return inputs[req.Addr : req.Addr+req.Quantity], nil
}

func (p *PLC) HandleHoldingRegisters(req *modbus.HoldingRegistersRequest) ([]uint16, error) {
	log := p.log.With("client", req.ClientAddr, "unit", req.UnitId, "table", "holding_registers",
		"addr", req.Addr, "qty", req.Quantity)
	if !inRange(req.Addr, req.Quantity, NumHoldingRegisters) {
		log.Warn("rejected: illegal data address", "write", req.IsWrite)
		return nil, modbus.ErrIllegalDataAddress
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	regs := p.sp.registers()
	if !req.IsWrite {
		log.Debug("read")
		return regs[req.Addr : req.Addr+req.Quantity], nil
	}

	next := slices.Clone(regs)
	copy(next[req.Addr:], req.Args)
	sp := setpointsFromRegisters(next)
	if err := sp.validate(p.tank.Config().Capacity); err != nil {
		log.Warn("rejected: illegal data value", "values", req.Args, "reason", err)
		return nil, modbus.ErrIllegalDataValue
	}

	p.sp = sp
	log.Info("write", "values", req.Args)
	return nil, nil
}

func (p *PLC) HandleInputRegisters(req *modbus.InputRegistersRequest) ([]uint16, error) {
	log := p.log.With("client", req.ClientAddr, "unit", req.UnitId, "table", "input_registers",
		"addr", req.Addr, "qty", req.Quantity)
	if !inRange(req.Addr, req.Quantity, NumInputRegisters) {
		log.Warn("rejected: illegal data address")
		return nil, modbus.ErrIllegalDataAddress
	}

	s := p.State()
	regs := []uint16{
		IRLevel:      toRegister(s.Level),
		IRLevelPct:   toRegister(100 * s.Level / s.Capacity),
		IRInflow:     toRegister(60 * s.Inflow),
		IROutflow:    toRegister(60 * s.Outflow),
		IRPumpStarts: toRegister(float64(s.PumpStarts)),
		IRSpilled:    toRegister(s.Spilled),
		IRCapacity:   toRegister(s.Capacity),
	}
	log.Debug("read")
	return regs[req.Addr : req.Addr+req.Quantity], nil
}

func inRange(addr, qty uint16, n int) bool {
	return qty > 0 && int(addr)+int(qty) <= n
}

func toRegister(v float64) uint16 {
	return uint16(math.Round(math.Min(math.Max(v, 0), math.MaxUint16)))
}
