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
	"os"

	"github.com/sirupsen/logrus"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/server"
)

func main() {
	log := logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetOutput(os.Stdout)

	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("load config")
	}
	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.WithError(err).Fatal("parse LOG_LEVEL")
	}
	log.SetLevel(level)

	entry := log.WithFields(logrus.Fields{
		"component":   "external-dns-netbird",
		"webhookAddr": cfg.WebhookAddr,
		"metricsAddr": cfg.MetricsAddr,
		"telemetry":   "disabled",
	})

	api := netbird.NewClient(cfg.BaseURL, cfg.PAT)
	p := provider.New(api, cfg.DomainFilter, cfg.DefaultTTL)
	srv := server.New(p, entry, cfg.WebhookAddr, cfg.MetricsAddr)
	entry.Info("starting")
	if err := srv.Run(); err != nil {
		entry.WithError(err).Fatal("server exited")
	}
}
