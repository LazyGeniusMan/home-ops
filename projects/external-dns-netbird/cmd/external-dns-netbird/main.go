// Command external-dns-netbird is an ExternalDNS webhook provider backed by
// NetBird DNS Custom Zones. It is intended to run as a localhost-only sidecar
// next to ExternalDNS (--provider=webhook).
//
// Configuration is taken from the environment; see internal/config. The
// NetBird personal access token is read from the file named by
// NETBIRD_PAT_FILE and never from a bare environment value.
//
// No telemetry is collected or transmitted: the binary performs no
// phone-home, update checks, or usage reporting of any kind. The only
// network traffic is NetBird Public API calls and the local webhook,
// health, and metrics listeners.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/server"
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("external-dns-netbird: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	// Load config before creating the levelled logger; last-resort errors
	// go to stderr via main.
	boot := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		boot.Error("load config", slog.Any("err", err))
		return err
	}
	level, err := parseLevel(cfg.LogLevel)
	if err != nil {
		boot.Error("parse LOG_LEVEL", slog.Any("err", err))
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With(
		slog.String("component", "external-dns-netbird"),
		slog.String("webhookAddr", cfg.WebhookAddr),
		slog.String("metricsAddr", cfg.MetricsAddr),
		slog.String("telemetry", "disabled"),
	)

	api := netbird.NewClient(cfg.BaseURL, cfg.PAT)
	p := provider.New(api, cfg.DomainFilter, cfg.DefaultTTL)
	srv := server.New(p, log, cfg.WebhookAddr, cfg.MetricsAddr)
	// signal.NotifyContext converts SIGINT/SIGTERM (docker stop) into
	// context cancellation so Server.Run drains both listeners gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Info("starting")
	if err := srv.Run(ctx); err != nil {
		log.Error("server exited", slog.Any("err", err))
		return err
	}
	return nil
}

// parseLevel maps LOG_LEVEL names (debug, info, warn/warning, error) to slog levels.
func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, &invalidLevelError{level: raw}
	}
}

type invalidLevelError struct{ level string }

func (e *invalidLevelError) Error() string {
	return "invalid LOG_LEVEL " + strconv.Quote(e.level) + ": want one of debug, info, warn, error"
}
