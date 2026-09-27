package tank

import (
	"math"
	"testing"
	"time"
)

func TestStep(t *testing.T) {
	cfg := DefaultConfig() // 1000 L, pump 20 L/s, drain 12 L/s

	tests := []struct {
		name        string
		level       float64
		dt          time.Duration
		pump, valve bool
		wantLevel   float64
		wantSpilled float64
		wantFlows   Flows
	}{
		{
			name:  "idle keeps level",
			level: 400, dt: time.Second,
			wantLevel: 400,
		},
		{
			name:  "pump fills",
			level: 400, dt: time.Second, pump: true,
			wantLevel: 420,
			wantFlows: Flows{In: 20},
		},
		{
			name:  "valve drains",
			level: 400, dt: time.Second, valve: true,
			wantLevel: 388,
			wantFlows: Flows{Out: 12},
		},
		{
			name:  "pump and valve net fill",
			level: 400, dt: 500 * time.Millisecond, pump: true, valve: true,
			wantLevel: 404,
			wantFlows: Flows{In: 20, Out: 12},
		},
		{
			name:  "drain stops at empty",
			level: 5, dt: time.Second, valve: true,
			wantLevel: 0,
			wantFlows: Flows{Out: 5},
		},
		{
			name:  "pump overflows full tank",
			level: 995, dt: time.Second, pump: true,
			wantLevel: 1000, wantSpilled: 15,
			wantFlows: Flows{In: 20, Overflowing: true},
		},
		{
			name:  "zero dt is a no-op",
			level: 400, dt: 0, pump: true, valve: true,
			wantLevel: 400,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tk, err := New(cfg, tc.level)
			if err != nil {
				t.Fatal(err)
			}
			got := tk.Step(tc.dt, tc.pump, tc.valve)

			if !approx(tk.Level(), tc.wantLevel) {
				t.Errorf("level = %.3f, want %.3f", tk.Level(), tc.wantLevel)
			}
			if !approx(tk.Spilled(), tc.wantSpilled) {
				t.Errorf("spilled = %.3f, want %.3f", tk.Spilled(), tc.wantSpilled)
			}
			if !approx(got.In, tc.wantFlows.In) || !approx(got.Out, tc.wantFlows.Out) ||
				got.Overflowing != tc.wantFlows.Overflowing {
				t.Errorf("flows = %+v, want %+v", got, tc.wantFlows)
			}
		})
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		cfg   Config
		level float64
	}{
		{"zero capacity", Config{Capacity: 0, PumpRate: 1, DrainRate: 1}, 0},
		{"negative pump rate", Config{Capacity: 10, PumpRate: -1, DrainRate: 1}, 0},
		{"negative drain rate", Config{Capacity: 10, PumpRate: 1, DrainRate: -1}, 0},
		{"negative level", DefaultConfig(), -1},
		{"level above capacity", DefaultConfig(), 1001},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg, tc.level); err == nil {
				t.Fatal("New succeeded, want error")
			}
		})
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
