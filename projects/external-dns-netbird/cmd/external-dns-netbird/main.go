// Command external-dns-netbird is an ExternalDNS webhook provider backed by
// NetBird DNS Custom Zones (localhost-only sidecar, --provider=webhook).
// Config from the environment (PAT via NETBIRD_PAT_FILE); traces via OTLP,
// metrics via Prometheus.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/server"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/tracing"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/version"
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
		slog.String("telemetry", "otlp"),
	)

	// Traces export to the in-namespace OTLP collector (OTEL_EXPORTER_OTLP_ENDPOINT);
	// Setup disables itself with OTEL_SDK_DISABLED=true.
	bootCtx := context.Background()
	shutdownTracing, err := tracing.Setup(bootCtx, "external-dns-netbird", version.Version)
	if err != nil {
		log.Warn("tracing disabled", slog.Any("err", err))
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

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
