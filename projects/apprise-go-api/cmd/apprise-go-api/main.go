// Command apprise-go-api is a stateless-only Go port of Python apprise-api:
// POST /notify, request-scoped attachments, webhook remap/callback. Config
// from the environment (secrets via *_FILE); traces via OTLP, metrics via
// Prometheus.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/server"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/tracing"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/version"
)

func main() {
	if err := run(); err != nil {
		// slog is unavailable before config loads; last-resort stderr.
		_, _ = os.Stderr.WriteString("apprise-go-api: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	} else {
		switch cfg.LogLevel {
		case "debug":
			level = slog.LevelDebug
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// Traces export to the infra otel-gateway OTLP collector (OTEL_EXPORTER_OTLP_ENDPOINT);
	// Setup disables itself with OTEL_SDK_DISABLED=true.
	shutdownTracing, err := tracing.Setup(context.Background(), "apprise-go-api", version.Version)
	if err != nil {
		log.Warn("tracing disabled", "err", err)
		shutdownTracing = func(context.Context) error { return nil }
	}

	sender := notify.New(time.Duration(cfg.CallTimeoutSecs) * time.Second)
	srv := server.New(cfg, sender, log)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    config.MaxHeaderBytes(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting", "addr", cfg.Addr, "stateless_storage", cfg.StatelessStorage, "telemetry", "otlp")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeoutSecs)*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	// Flush buffered spans before exit (bounded by the drain timeout).
	_ = shutdownTracing(shutdownCtx)
	// Shutdown drains in-flight requests; docker stop waits for this.
	log.Info("drained")
	return <-errCh
}
