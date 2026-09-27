package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simonvetter/modbus"

	"github.com/mourad-boussiouf-epitech/modguard/internal/plc"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:5020", "Modbus/TCP listen address")
	scan := flag.Duration("scan", 100*time.Millisecond, "PLC scan cycle period")
	status := flag.Duration("status", 5*time.Second, "how often to log the tank state (0 disables)")
	verbose := flag.Bool("v", false, "also log every read request")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := run(log, *listen, *scan, *status); err != nil {
		log.Error("plcsim failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, listen string, scan, status time.Duration) error {
	if scan <= 0 {
		return fmt.Errorf("scan period must be positive, got %v", scan)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p, err := plc.New(plc.DefaultConfig(), log)
	if err != nil {
		return err
	}

	srv, err := modbus.NewServer(&modbus.ServerConfiguration{
		URL:        "tcp://" + listen,
		Timeout:    time.Minute,
		MaxClients: 16,
		Logger:     slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}, p)
	if err != nil {
		return err
	}
	if err := srv.Start(); err != nil {
		return fmt.Errorf("listen on %s: %w", listen, err)
	}
	defer srv.Stop()

	log.Info("PLC simulator running", "listen", listen, "scan", scan)
	if status > 0 {
		go logStatus(ctx, log, p, status)
	}

	p.Run(ctx, scan)
	log.Info("shutting down")
	return nil
}

func logStatus(ctx context.Context, log *slog.Logger, p *plc.PLC, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := p.State()
			log.Info("tank",
				"level_l", int(s.Level+0.5),
				"pump", onOff(s.Pump),
				"valve", openClosed(s.Valve),
				"auto", onOff(s.Auto),
				"low_alarm", s.LowAlarm,
				"high_alarm", s.HighAlarm,
				"spilled_l", int(s.Spilled+0.5))
		}
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func openClosed(b bool) string {
	if b {
		return "open"
	}
	return "closed"
}
