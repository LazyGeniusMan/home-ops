// Command eso-proton-pass is the ESO webhook provider for Proton Pass
// (pull-only; push returns 501). Secrets: pass://{vault}/{item}/{field};
// PAT from PROTON_PASS_PAT_FILE. Traces via OTLP, metrics via Prometheus.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/passclient"
	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/server"
	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/tracing"
	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/version"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "eso-proton-pass: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Traces export to the infra otel-gateway OTLP collector (OTEL_EXPORTER_OTLP_ENDPOINT);
	// Setup disables itself with OTEL_SDK_DISABLED=true.
	shutdownTracing, err := tracing.Setup(context.Background(), "eso-proton-pass", version.Version)
	if err != nil {
		logger.Warn("tracing disabled", slog.Any("err", err))
		shutdownTracing = func(context.Context) error { return nil }
	}

	pat, err := passclient.ReadPATFile(cfg.PATFile)
	if err != nil {
		return err
	}

	client := passclient.New(passclient.Options{
		BinaryPath: cfg.PassCLIBinary,
		SessionDir: cfg.SessionDir,
		Timeout:    cfg.ExecTimeout,
		Logger:     logger,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := client.Login(ctx, pat); err != nil {
		return fmt.Errorf("pass-cli login: %w", err)
	}

	prov := provider.New(client, logger)
	srv := server.New(prov, logger)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", cfg.ListenAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		// SIGINT/SIGTERM (docker stop) drains in-flight requests before the
		// process exits: bounded Shutdown lets handlers finish.
		logger.Info("shutting down", slog.String("reason", ctx.Err().Error()))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		// Flush buffered spans before exit (bounded by the drain timeout).
		_ = shutdownTracing(shutdownCtx)
		logger.Info("drained")
		return nil
	case err := <-errCh:
		// Listener failed: shut down gracefully before returning so no
		// in-flight request is orphaned.
		if err != nil {
			logger.Error("listener failed, shutting down", slog.Any("err", err))
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		// Flush buffered spans before exit (bounded by the drain timeout).
		_ = shutdownTracing(shutdownCtx)
		logger.Info("drained")
		return err
	}
}
