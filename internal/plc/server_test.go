package plc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/simonvetter/modbus"
)

func TestOverTCP(t *testing.T) {
	p := newTestPLC(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, 10*time.Millisecond)

	addr := freeAddr(t)
	srv, err := modbus.NewServer(&modbus.ServerConfiguration{
		URL: "tcp://" + addr, Timeout: 5 * time.Second, MaxClients: 4,
	}, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, err := modbus.NewClient(&modbus.ClientConfiguration{URL: "tcp://" + addr, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Open(); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	capacity, err := c.ReadRegister(IRCapacity, modbus.INPUT_REGISTER)
	if err != nil || capacity != 1000 {
		t.Fatalf("capacity = %d, %v; want 1000", capacity, err)
	}

	first, err := c.ReadRegister(IRLevel, modbus.INPUT_REGISTER)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	second, err := c.ReadRegister(IRLevel, modbus.INPUT_REGISTER)
	if err != nil {
		t.Fatal(err)
	}
	if second >= first {
		t.Errorf("level did not fall with the drain open: %d L then %d L", first, second)
	}

	if err := c.WriteCoil(CoilValve, false); err != nil {
		t.Fatal(err)
	}
	if open, err := c.ReadDiscreteInput(DIValveOpen); err != nil || open {
		t.Errorf("valve open = %v, %v; want false", open, err)
	}
	if err := c.WriteRegister(HRPumpOnLevel, 250); err != nil {
		t.Fatal(err)
	}
	if v, err := c.ReadRegister(HRPumpOnLevel, modbus.HOLDING_REGISTER); err != nil || v != 250 {
		t.Errorf("pump-on level = %d, %v; want 250", v, err)
	}

	if err := c.WriteCoil(CoilPump, true); !errors.Is(err, modbus.ErrIllegalDataValue) {
		t.Errorf("pump write in auto mode: err = %v, want illegal data value", err)
	}
	if err := c.WriteRegister(HRHighAlarm, 5000); !errors.Is(err, modbus.ErrIllegalDataValue) {
		t.Errorf("setpoint above capacity: err = %v, want illegal data value", err)
	}
	if _, err := c.ReadRegisters(0, 10, modbus.HOLDING_REGISTER); !errors.Is(err, modbus.ErrIllegalDataAddress) {
		t.Errorf("read past holding table: err = %v, want illegal data address", err)
	}

	if _, err := c.ReadCoils(0, NumCoils); err != nil {
		t.Errorf("read after exceptions: %v", err)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}
