package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/simonvetter/modbus"

	"github.com/mourad-boussiouf-epitech/modguard/internal/plc"
)

const usageText = `Usage: plcclient [flags] <command> [args]

Monitoring:
  status                         print the tank state once
  watch                          live dashboard (Ctrl-C to quit)

Operator commands:
  auto on|off                    automatic level control
  pump on|off                    pump (auto mode must be off)
  valve open|close               drain valve
  setpoint pump-on|pump-off <L>  auto mode start/stop levels
  alarm low|high <L>             alarm thresholds

Raw Modbus access:
  read coils|di|hr|ir <addr> [qty]
  write coil <addr> 0|1
  write hr <addr> <value>

Flags:
`

func main() {
	addr := flag.String("addr", "127.0.0.1:5020", "PLC address (host:port)")
	unit := flag.Uint("unit", 1, "Modbus unit ID")
	timeout := flag.Duration("timeout", 2*time.Second, "request timeout")
	interval := flag.Duration("interval", 500*time.Millisecond, "refresh period for watch")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usageText)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *unit > 255 {
		fmt.Fprintln(os.Stderr, "plcclient: unit ID must be 0-255")
		os.Exit(2)
	}

	if err := run(*addr, uint8(*unit), *timeout, *interval, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "plcclient:", describe(err))
		os.Exit(1)
	}
}

func run(addr string, unit uint8, timeout, interval time.Duration, args []string) error {
	c, err := modbus.NewClient(&modbus.ClientConfiguration{URL: "tcp://" + addr, Timeout: timeout})
	if err != nil {
		return err
	}
	if err := c.SetUnitId(unit); err != nil {
		return err
	}
	if err := c.Open(); err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer c.Close()

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "status":
		v, err := readView(c)
		if err != nil {
			return err
		}
		render(os.Stdout, addr, v)
		return nil
	case "watch":
		return watch(c, addr, interval)
	case "auto":
		return writeSwitch(c, plc.CoilAuto, rest, "on", "off")
	case "pump":
		err := writeSwitch(c, plc.CoilPump, rest, "on", "off")
		if errors.Is(err, modbus.ErrIllegalDataValue) {
			err = fmt.Errorf("%w: auto mode owns the pump, run 'plcclient auto off' first", err)
		}
		return err
	case "valve":
		return writeSwitch(c, plc.CoilValve, rest, "open", "close")
	case "setpoint":
		return writeThreshold(c, rest, map[string]uint16{"pump-on": plc.HRPumpOnLevel, "pump-off": plc.HRPumpOffLevel})
	case "alarm":
		return writeThreshold(c, rest, map[string]uint16{"low": plc.HRLowAlarm, "high": plc.HRHighAlarm})
	case "read":
		return rawRead(c, rest)
	case "write":
		return rawWrite(c, rest)
	default:
		return fmt.Errorf("unknown command %q (run with -h for help)", cmd)
	}
}

type view struct {
	coils    []bool
	inputs   []bool
	measures []uint16
	setpts   []uint16
}

func readView(c *modbus.ModbusClient) (view, error) {
	var v view
	var err error
	if v.coils, err = c.ReadCoils(0, plc.NumCoils); err != nil {
		return v, fmt.Errorf("read coils: %w", err)
	}
	if v.inputs, err = c.ReadDiscreteInputs(0, plc.NumDiscreteInputs); err != nil {
		return v, fmt.Errorf("read discrete inputs: %w", err)
	}
	if v.measures, err = c.ReadRegisters(0, plc.NumInputRegisters, modbus.INPUT_REGISTER); err != nil {
		return v, fmt.Errorf("read input registers: %w", err)
	}
	if v.setpts, err = c.ReadRegisters(0, plc.NumHoldingRegisters, modbus.HOLDING_REGISTER); err != nil {
		return v, fmt.Errorf("read holding registers: %w", err)
	}
	return v, nil
}

func watch(c *modbus.ModbusClient, addr string, interval time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		v, err := readView(c)
		if err != nil {
			return err
		}
		fmt.Print("\033[H\033[2J") // clear screen
		render(os.Stdout, addr, v)
		fmt.Println("\n  Ctrl-C to quit")

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func render(w io.Writer, addr string, v view) {
	level := int(v.measures[plc.IRLevel])
	capacity := int(v.measures[plc.IRCapacity])

	const width = 40
	filled := 0
	if capacity > 0 {
		filled = min(width, width*level/capacity)
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	mode := "manual"
	if v.coils[plc.CoilAuto] {
		mode = "auto"
	}

	fmt.Fprintf(w, "Water tank PLC @ %s\n\n", addr)
	fmt.Fprintf(w, "  level      [%s] %4d / %d L (%d%%)\n", bar, level, capacity, v.measures[plc.IRLevelPct])
	fmt.Fprintf(w, "  mode       %s\n", mode)
	fmt.Fprintf(w, "  pump       %-8s inflow  %5d L/min\n", word(v.inputs[plc.DIPumpRunning], "ON", "off"), v.measures[plc.IRInflow])
	fmt.Fprintf(w, "  valve      %-8s outflow %5d L/min\n", word(v.inputs[plc.DIValveOpen], "OPEN", "closed"), v.measures[plc.IROutflow])
	fmt.Fprintf(w, "  alarms     low %s   high %s   overflow %s\n",
		word(v.inputs[plc.DILowAlarm], "ALARM", "ok"),
		word(v.inputs[plc.DIHighAlarm], "ALARM", "ok"),
		word(v.inputs[plc.DIOverflow], "ALARM", "ok"))
	fmt.Fprintf(w, "  setpoints  pump on <= %d L, off >= %d L; alarms low <= %d L, high >= %d L\n",
		v.setpts[plc.HRPumpOnLevel], v.setpts[plc.HRPumpOffLevel], v.setpts[plc.HRLowAlarm], v.setpts[plc.HRHighAlarm])
	fmt.Fprintf(w, "  counters   pump starts %d, spilled %d L\n", v.measures[plc.IRPumpStarts], v.measures[plc.IRSpilled])
}

func word(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func writeSwitch(c *modbus.ModbusClient, coil uint16, args []string, on, off string) error {
	if len(args) != 1 || (args[0] != on && args[0] != off) {
		return fmt.Errorf("expected %s or %s", on, off)
	}
	if err := c.WriteCoil(coil, args[0] == on); err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func writeThreshold(c *modbus.ModbusClient, args []string, names map[string]uint16) error {
	if len(args) != 2 {
		return errors.New("expected <name> <litres>")
	}
	reg, ok := names[args[0]]
	if !ok {
		return fmt.Errorf("unknown name %q", args[0])
	}
	value, err := parseUint16(args[1])
	if err != nil {
		return err
	}
	err = c.WriteRegister(reg, value)
	if errors.Is(err, modbus.ErrIllegalDataValue) {
		return fmt.Errorf("%w: thresholds must be within capacity, with pump-on below pump-off and low alarm below high alarm", err)
	}
	if err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func rawRead(c *modbus.ModbusClient, args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return errors.New("expected read coils|di|hr|ir <addr> [qty]")
	}
	addr, err := parseUint16(args[1])
	if err != nil {
		return err
	}
	qty := uint16(1)
	if len(args) == 3 {
		if qty, err = parseUint16(args[2]); err != nil {
			return err
		}
	}

	switch args[0] {
	case "coils", "di":
		read := c.ReadCoils
		if args[0] == "di" {
			read = c.ReadDiscreteInputs
		}
		values, err := read(addr, qty)
		if err != nil {
			return err
		}
		for i, b := range values {
			fmt.Printf("%s[%d] = %d\n", args[0], int(addr)+i, boolToInt(b))
		}
	case "hr", "ir":
		regType := modbus.HOLDING_REGISTER
		if args[0] == "ir" {
			regType = modbus.INPUT_REGISTER
		}
		values, err := c.ReadRegisters(addr, qty, regType)
		if err != nil {
			return err
		}
		for i, r := range values {
			fmt.Printf("%s[%d] = %d\n", args[0], int(addr)+i, r)
		}
	default:
		return fmt.Errorf("unknown table %q (want coils, di, hr or ir)", args[0])
	}
	return nil
}

func rawWrite(c *modbus.ModbusClient, args []string) error {
	if len(args) != 3 {
		return errors.New("expected write coil <addr> 0|1 or write hr <addr> <value>")
	}
	addr, err := parseUint16(args[1])
	if err != nil {
		return err
	}
	value, err := parseUint16(args[2])
	if err != nil {
		return err
	}

	switch args[0] {
	case "coil":
		if value > 1 {
			return errors.New("coil value must be 0 or 1")
		}
		err = c.WriteCoil(addr, value == 1)
	case "hr":
		err = c.WriteRegister(addr, value)
	default:
		return fmt.Errorf("unknown table %q (want coil or hr)", args[0])
	}
	if err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func describe(err error) string {
	exceptions := []struct {
		err  error
		code byte
	}{
		{modbus.ErrIllegalFunction, 0x01},
		{modbus.ErrIllegalDataAddress, 0x02},
		{modbus.ErrIllegalDataValue, 0x03},
		{modbus.ErrServerDeviceFailure, 0x04},
	}
	for _, e := range exceptions {
		if errors.Is(err, e.err) {
			return fmt.Sprintf("PLC refused the request with Modbus exception 0x%02X (%v)", e.code, err)
		}
	}
	return err.Error()
}

func parseUint16(s string) (uint16, error) {
	v, err := strconv.ParseUint(s, 0, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number between 0 and 65535", s)
	}
	return uint16(v), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
