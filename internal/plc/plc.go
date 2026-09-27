// Package plc simulates a PLC controlling a water tank, served over Modbus/TCP.
package plc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/mourad-boussiouf-epitech/modguard/internal/tank"
)

// Setpoints are in litres.
type Setpoints struct {
	PumpOn    uint16
	PumpOff   uint16
	LowAlarm  uint16
	HighAlarm uint16
}

func (s Setpoints) validate(capacity float64) error {
	for _, v := range []uint16{s.PumpOn, s.PumpOff, s.LowAlarm, s.HighAlarm} {
		if float64(v) > capacity {
			return fmt.Errorf("setpoint %d L exceeds capacity %.0f L", v, capacity)
		}
	}
	if s.PumpOn >= s.PumpOff {
		return fmt.Errorf("pump-on level %d L must be below pump-off level %d L", s.PumpOn, s.PumpOff)
	}
	if s.LowAlarm >= s.HighAlarm {
		return fmt.Errorf("low alarm %d L must be below high alarm %d L", s.LowAlarm, s.HighAlarm)
	}
	return nil
}

func (s Setpoints) registers() []uint16 {
	return []uint16{
		HRPumpOnLevel:  s.PumpOn,
		HRPumpOffLevel: s.PumpOff,
		HRLowAlarm:     s.LowAlarm,
		HRHighAlarm:    s.HighAlarm,
	}
}

func setpointsFromRegisters(r []uint16) Setpoints {
	return Setpoints{
		PumpOn:    r[HRPumpOnLevel],
		PumpOff:   r[HRPumpOffLevel],
		LowAlarm:  r[HRLowAlarm],
		HighAlarm: r[HRHighAlarm],
	}
}

type Config struct {
	Tank         tank.Config
	InitialLevel float64
	Setpoints    Setpoints
	Auto         bool
	ValveOpen    bool
}

func DefaultConfig() Config {
	return Config{
		Tank:         tank.DefaultConfig(),
		InitialLevel: 500,
		Setpoints:    Setpoints{PumpOn: 300, PumpOff: 800, LowAlarm: 100, HighAlarm: 900},
		Auto:         true,
		ValveOpen:    true,
	}
}

type State struct {
	Level      float64 // L
	Capacity   float64 // L
	Pump       bool
	Valve      bool
	Auto       bool
	LowAlarm   bool
	HighAlarm  bool
	Overflow   bool
	Inflow     float64 // L/s
	Outflow    float64 // L/s
	PumpStarts uint64
	Spilled    float64 // L
	Setpoints  Setpoints
}

type PLC struct {
	log *slog.Logger

	mu         sync.Mutex
	tank       *tank.Tank
	pump       bool
	valve      bool
	auto       bool
	sp         Setpoints
	flows      tank.Flows
	pumpStarts uint64
}

func New(cfg Config, log *slog.Logger) (*PLC, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if cfg.Tank.Capacity > math.MaxUint16 {
		return nil, errors.New("plc: capacity must fit in a 16-bit register")
	}
	t, err := tank.New(cfg.Tank, cfg.InitialLevel)
	if err != nil {
		return nil, err
	}
	if err := cfg.Setpoints.validate(cfg.Tank.Capacity); err != nil {
		return nil, fmt.Errorf("plc: %w", err)
	}
	return &PLC{
		log:   log,
		tank:  t,
		valve: cfg.ValveOpen,
		auto:  cfg.Auto,
		sp:    cfg.Setpoints,
	}, nil
}

// Run advances the simulation by a fixed period per tick, so it pauses
// instead of jumping ahead if the laptop sleeps.
func (p *PLC) Run(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Scan(period)
		}
	}
}

func (p *PLC) Scan(dt time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.auto {
		level := p.tank.Level()
		switch {
		case level <= float64(p.sp.PumpOn):
			p.setPump(true)
		case level >= float64(p.sp.PumpOff):
			p.setPump(false)
		}
	}
	p.flows = p.tank.Step(dt, p.pump, p.valve)
}

func (p *PLC) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stateLocked()
}

func (p *PLC) stateLocked() State {
	level := p.tank.Level()
	return State{
		Level:      level,
		Capacity:   p.tank.Config().Capacity,
		Pump:       p.pump,
		Valve:      p.valve,
		Auto:       p.auto,
		LowAlarm:   level <= float64(p.sp.LowAlarm),
		HighAlarm:  level >= float64(p.sp.HighAlarm),
		Overflow:   p.flows.Overflowing,
		Inflow:     p.flows.In,
		Outflow:    p.flows.Out,
		PumpStarts: p.pumpStarts,
		Spilled:    p.tank.Spilled(),
		Setpoints:  p.sp,
	}
}

func (p *PLC) setPump(on bool) {
	if on && !p.pump {
		p.pumpStarts++
	}
	p.pump = on
}
