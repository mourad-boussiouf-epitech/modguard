// Package tank simulates the water level of a tank filled by a pump and emptied by a drain valve.
package tank

import (
	"errors"
	"fmt"
	"time"
)

type Config struct {
	Capacity  float64 // L
	PumpRate  float64 // L/s
	DrainRate float64 // L/s
}

func DefaultConfig() Config {
	return Config{Capacity: 1000, PumpRate: 20, DrainRate: 12}
}

func (c Config) Validate() error {
	switch {
	case c.Capacity <= 0:
		return errors.New("tank: capacity must be positive")
	case c.PumpRate < 0:
		return errors.New("tank: pump rate must not be negative")
	case c.DrainRate < 0:
		return errors.New("tank: drain rate must not be negative")
	}
	return nil
}

type Flows struct {
	In          float64 // L/s
	Out         float64 // L/s
	Overflowing bool
}

type Tank struct {
	cfg     Config
	level   float64
	spilled float64
}

func New(cfg Config, level float64) (*Tank, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if level < 0 || level > cfg.Capacity {
		return nil, fmt.Errorf("tank: initial level %.1f L outside [0, %.1f]", level, cfg.Capacity)
	}
	return &Tank{cfg: cfg, level: level}, nil
}

func (t *Tank) Config() Config   { return t.cfg }
func (t *Tank) Level() float64   { return t.level }
func (t *Tank) Spilled() float64 { return t.spilled }

func (t *Tank) Step(dt time.Duration, pumpOn, valveOpen bool) Flows {
	sec := dt.Seconds()
	if sec <= 0 {
		return Flows{}
	}

	var in, out float64
	if pumpOn {
		in = t.cfg.PumpRate
	}
	if valveOpen {
		out = t.cfg.DrainRate
	}

	level := t.level + (in-out)*sec
	if level < 0 {
		// ran dry: only drain what was there
		out = (t.level + in*sec) / sec
		level = 0
	}

	overflowing := false
	if level > t.cfg.Capacity {
		t.spilled += level - t.cfg.Capacity
		level = t.cfg.Capacity
		overflowing = true
	}

	t.level = level
	return Flows{In: in, Out: out, Overflowing: overflowing}
}
