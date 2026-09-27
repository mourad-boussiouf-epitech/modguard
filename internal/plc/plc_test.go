package plc

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/simonvetter/modbus"
)

func newTestPLC(t *testing.T, mutate func(*Config)) *PLC {
	t.Helper()
	cfg := DefaultConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	p, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutoModeHysteresis(t *testing.T) {
	p := newTestPLC(t, func(c *Config) { c.InitialLevel = 310 })

	var starts, stops []float64
	wasOn := false
	for range 400 {
		before := p.State().Level
		p.Scan(time.Second)
		s := p.State()
		if s.Pump && !wasOn {
			starts = append(starts, before)
		}
		if !s.Pump && wasOn {
			stops = append(stops, before)
		}
		wasOn = s.Pump
		if s.Level < 290 || s.Level > 810 {
			t.Fatalf("level %.1f L left the control band", s.Level)
		}
	}

	if len(starts) < 2 || len(stops) < 2 {
		t.Fatalf("expected the pump to cycle, got starts at %v, stops at %v", starts, stops)
	}
	for _, l := range starts {
		if l > 300 {
			t.Errorf("pump started at %.1f L, want <= 300 L", l)
		}
	}
	for _, l := range stops {
		if l < 800 {
			t.Errorf("pump stopped at %.1f L, want >= 800 L", l)
		}
	}
	if got := p.State().PumpStarts; got != uint64(len(starts)) {
		t.Errorf("PumpStarts = %d, want %d", got, len(starts))
	}
}

func TestManualPumpOverflows(t *testing.T) {
	p := newTestPLC(t, nil)
	mustWriteCoils(t, p, CoilPump, true, false, false)

	for range 60 {
		p.Scan(time.Second)
	}
	s := p.State()
	if s.Level != s.Capacity || !s.Overflow || !s.HighAlarm {
		t.Fatalf("want a full, overflowing tank with high alarm, got %+v", s)
	}
	if want := 500 + 60*20 - 1000.0; s.Spilled != want {
		t.Errorf("spilled = %.1f L, want %.1f L", s.Spilled, want)
	}
}

func TestHandleCoils(t *testing.T) {
	tests := []struct {
		name    string
		addr    uint16
		args    []bool
		wantErr error
		want    []bool
	}{
		{"close valve", CoilValve, []bool{false}, nil, []bool{false, false, true}},
		{"pump write refused in auto mode", CoilPump, []bool{true}, modbus.ErrIllegalDataValue, []bool{false, true, true}},
		{"leave auto mode", CoilAuto, []bool{false}, nil, []bool{false, true, false}},
		{"auto off and pump on in one request", CoilPump, []bool{true, true, false}, nil, []bool{true, true, false}},
		{"unchanged pump value accepted in auto", CoilPump, []bool{false, false, true}, nil, []bool{false, false, true}},
		{"address past the table", CoilAuto, []bool{true, true}, modbus.ErrIllegalDataAddress, []bool{false, true, true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPLC(t, nil)
			_, err := p.HandleCoils(&modbus.CoilsRequest{
				Addr: tc.addr, Quantity: uint16(len(tc.args)), IsWrite: true, Args: tc.args,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			got, err := p.HandleCoils(&modbus.CoilsRequest{Addr: 0, Quantity: NumCoils})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("coils = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHandleHoldingRegisters(t *testing.T) {
	defaults := []uint16{300, 800, 100, 900}
	tests := []struct {
		name    string
		addr    uint16
		args    []uint16
		wantErr error
		want    []uint16
	}{
		{"single write", HRPumpOnLevel, []uint16{250}, nil, []uint16{250, 800, 100, 900}},
		{"multi write", HRPumpOnLevel, []uint16{200, 700, 50, 950}, nil, []uint16{200, 700, 50, 950}},
		{"above capacity", HRHighAlarm, []uint16{1001}, modbus.ErrIllegalDataValue, defaults},
		{"pump-on not below pump-off", HRPumpOnLevel, []uint16{800}, modbus.ErrIllegalDataValue, defaults},
		{"one bad value rejects the whole write", HRPumpOnLevel, []uint16{200, 700, 950, 50}, modbus.ErrIllegalDataValue, defaults},
		{"address past the table", HRHighAlarm, []uint16{1, 2}, modbus.ErrIllegalDataAddress, defaults},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPLC(t, nil)
			_, err := p.HandleHoldingRegisters(&modbus.HoldingRegistersRequest{
				Addr: tc.addr, Quantity: uint16(len(tc.args)), IsWrite: true, Args: tc.args,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			got, err := p.HandleHoldingRegisters(&modbus.HoldingRegistersRequest{Addr: 0, Quantity: NumHoldingRegisters})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("registers = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadOnlyTables(t *testing.T) {
	p := newTestPLC(t, nil)

	ir, err := p.HandleInputRegisters(&modbus.InputRegistersRequest{Addr: 0, Quantity: NumInputRegisters})
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint16{500, 50, 0, 0, 0, 0, 1000}; !slices.Equal(ir, want) {
		t.Errorf("input registers = %v, want %v", ir, want)
	}

	p.Scan(time.Second)
	flow, err := p.HandleInputRegisters(&modbus.InputRegistersRequest{Addr: IROutflow, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if flow[0] != 720 {
		t.Errorf("outflow = %d L/min, want 720", flow[0])
	}

	di, err := p.HandleDiscreteInputs(&modbus.DiscreteInputsRequest{Addr: 0, Quantity: NumDiscreteInputs})
	if err != nil {
		t.Fatal(err)
	}
	if want := []bool{false, true, false, false, false}; !slices.Equal(di, want) {
		t.Errorf("discrete inputs = %v, want %v", di, want)
	}

	for _, addr := range []uint16{NumInputRegisters, 100} {
		if _, err := p.HandleInputRegisters(&modbus.InputRegistersRequest{Addr: addr, Quantity: 1}); !errors.Is(err, modbus.ErrIllegalDataAddress) {
			t.Errorf("IR addr %d: err = %v, want illegal data address", addr, err)
		}
	}
	if _, err := p.HandleDiscreteInputs(&modbus.DiscreteInputsRequest{Addr: 0, Quantity: NumDiscreteInputs + 1}); !errors.Is(err, modbus.ErrIllegalDataAddress) {
		t.Errorf("DI overlong read: err = %v, want illegal data address", err)
	}
}

func TestNewRejectsBadSetpoints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Setpoints.PumpOn = cfg.Setpoints.PumpOff
	if _, err := New(cfg, nil); err == nil {
		t.Fatal("New accepted pump-on == pump-off")
	}
}

func mustWriteCoils(t *testing.T, p *PLC, addr uint16, values ...bool) {
	t.Helper()
	_, err := p.HandleCoils(&modbus.CoilsRequest{
		Addr: addr, Quantity: uint16(len(values)), IsWrite: true, Args: values,
	})
	if err != nil {
		t.Fatal(err)
	}
}
