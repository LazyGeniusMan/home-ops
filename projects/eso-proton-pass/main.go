// Command eso-proton-pass is the External Secrets Operator (ESO) webhook
// provider for Proton Pass.
//
// It is a pull-only HTTP service: ESO's generic webhook provider issues GET
// (and HEAD for Validate) requests and resolves secrets with the pass-cli
// backend. Push operations are not implemented and return an explicit
// TODO/unimplemented response.
//
// Secret addressing (see README.md):
//
//	pass://{vault}/{item}/{field}
//
// Authentication:
//
//	PROTON_PASS_PAT_FILE  path to a file whose content is the Proton Pass
//	                      personal access token (PAT)
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

	"github.com/LazyGeniusMan/home-ops/eso-proton-pass/internal/config"
	"github.com/LazyGeniusMan/home-ops/eso-proton-pass/internal/passclient"
	"github.com/LazyGeniusMan/home-ops/eso-proton-pass/internal/provider"
	"github.com/LazyGeniusMan/home-ops/eso-proton-pass/internal/server"
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

	pat, err := passclient.ReadPATFile(cfg.PATFile)
	if err != nil {
		return err
	}

	client := passclient.New(passclient.Options{
		BinaryPath: cfg.PassCLIBinary,
		SessionDir: cfg.SessionDir,
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
