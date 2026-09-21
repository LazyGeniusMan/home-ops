// Command apprise-go-api is a stateless-only Go port of Python apprise-api.
//
// Supported features: stateless POST /notify, request-scoped attachments,
// and third-party webhook remap/callback. There is no persistent storage.
//
// Configuration is taken from the environment; see internal/config. Secrets
// are read from files named by *_FILE variables, never from bare
// environment values.
//
// No telemetry is collected or transmitted: the binary performs no
// phone-home, update checks, or usage reporting of any kind. The only
// network traffic is outbound notification delivery, the optional result
// webhook, and the local HTTP listener.
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
		log.Info("starting", "addr", cfg.Addr, "stateless_storage", cfg.StatelessStorage, "telemetry", "disabled")
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
	return <-errCh
}
