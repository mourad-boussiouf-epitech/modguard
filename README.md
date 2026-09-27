# modguard

An application-layer firewall for Modbus/TCP, written in Go. **Work in progress.**

This first step is the lab: a simulated PLC controlling a water tank, and an
operator console to watch and drive it. No hardware needed; everything runs on
a laptop. The firewall proxy will sit between the two later:

```
[plcclient]  ──TCP──▶  [modguard :5021]  ──TCP──▶  [plcsim :5020]
 operator              (coming next)               simulated PLC
```

## Try it

Requires Go 1.24 or newer (`brew install go` on macOS).

```sh
# Terminal 1: start the simulated PLC (logs every write and every refused request)
go run ./cmd/plcsim

# Terminal 2: live dashboard
go run ./cmd/plcclient watch

# Terminal 3: operate the plant
go run ./cmd/plcclient status
go run ./cmd/plcclient valve close
go run ./cmd/plcclient setpoint pump-off 700
go run ./cmd/plcclient auto off
go run ./cmd/plcclient pump on
```

On startup the tank is half full, in auto mode, with the drain open. The level
falls to 300 L, the PLC starts the pump, the level rises to 800 L, the PLC stops
the pump, and so on. One full cycle takes about 100 seconds.

Things to try:

- `plcclient pump on` while in auto mode: the PLC answers with Modbus exception
  0x03 (illegal data value), because auto mode owns the pump.
- `plcclient read hr 0 10`: exception 0x02 (illegal data address), because the
  holding register table only has 4 entries.
- `plcclient auto off`, `plcclient valve close`, `plcclient pump on`, then watch:
  the tank fills, raises the high alarm, then overflows and the spilled-litres
  counter climbs. The PLC obeys any client that can reach it, because Modbus
  has no authentication. That is the gap modguard will close.

`go run ./cmd/plcclient -h` lists every command, including raw
`read`/`write` access to any table and address.

## The simulated plant

| Part         | Behaviour                                              |
|--------------|--------------------------------------------------------|
| Tank         | 1000 L                                                 |
| Pump         | adds 20 L/s (1200 L/min) while running                 |
| Drain valve  | removes 12 L/s (720 L/min) while open                  |
| Auto mode    | pump on at or below the pump-on level, off at or above the pump-off level |
| Scan cycle   | 100 ms: read level, run the control program, drive the pump |

Writes take effect at the next scan, as on a real PLC. There is no overflow
interlock on purpose.

## Register map

Addresses are 0-based, as on the wire. The unit ID is ignored.

**Coils** (FC 01 read, FC 05/15 write)

| Addr | Name  | Meaning |
|------|-------|---------|
| 0    | pump  | pump output; writes refused (0x03) while auto mode is on |
| 1    | valve | drain valve, 1 = open |
| 2    | auto  | 1 = the PLC controls the pump |

**Discrete inputs** (FC 02, read-only)

| Addr | Name         | Meaning |
|------|--------------|---------|
| 0    | pump running | |
| 1    | valve open   | |
| 2    | low alarm    | level ≤ low alarm threshold |
| 3    | high alarm   | level ≥ high alarm threshold |
| 4    | overflow     | tank full and losing water |

**Input registers** (FC 04, read-only)

| Addr | Name        | Unit |
|------|-------------|------|
| 0    | level       | L |
| 1    | level       | % of capacity |
| 2    | inflow      | L/min |
| 3    | outflow     | L/min |
| 4    | pump starts | count since boot |
| 5    | spilled     | L since boot |
| 6    | capacity    | L |

**Holding registers** (FC 03 read, FC 06/16 write), all in litres

| Addr | Name           | Default |
|------|----------------|---------|
| 0    | pump-on level  | 300 |
| 1    | pump-off level | 800 |
| 2    | low alarm      | 100 |
| 3    | high alarm     | 900 |

Writes must keep every value within capacity, pump-on below pump-off, and low
alarm below high alarm. Otherwise the whole write is refused with exception
0x03 and nothing changes.

The simulator also works with any standard Modbus tool, for example
[mbpoll](https://github.com/epsilonrt/mbpoll) (`brew install mbpoll`):

```sh
mbpoll -m tcp -p 5020 -0 -t 3 -r 0 -c 7 -1 127.0.0.1   # read the 7 input registers
```

## Layout

```
cmd/plcsim/       simulated PLC: Modbus/TCP server on 127.0.0.1:5020
cmd/plcclient/    operator console: status, watch, commands, raw reads/writes
internal/tank/    the physical process (water level physics), no Modbus
internal/plc/     scan cycle, control program, register map, Modbus handler
```

## Tests

```sh
go test -race ./...
```

Table-driven tests cover the tank physics, the auto-mode hysteresis, the write
rules, and a full round trip through a real Modbus/TCP server and client.
