// Package config loads the webhook provider configuration from the
// environment. Secrets are never taken from the environment directly:
// the NetBird personal access token is read from the file whose path is
// given in NETBIRD_PAT_FILE (suitable for Kubernetes projected volumes,
// ESO secret mounts, etc.).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultBaseURL     = "https://api.netbird.io"
	defaultWebhookAddr = "127.0.0.1:8888"
	defaultMetricsAddr = ":8080"
	defaultTTL         = 300
	defaultLogLevel    = "info"
)

// Config is the runtime configuration of the webhook provider.
type Config struct {
	// PAT is the NetBird personal access token, read from PATFile.
	PAT string
	// PATFile is the path to the file holding the PAT (NETBIRD_PAT_FILE).
	PATFile string
	// BaseURL is the NetBird Public API base URL (NETBIRD_BASE_URL).
	BaseURL string
	// DomainFilter limits which DNS zones are served (DOMAIN_FILTER, comma-separated).
	DomainFilter []string
	// WebhookAddr is the localhost-only listen address for the webhook API (WEBHOOK_ADDR).
	WebhookAddr string
	// MetricsAddr is the listen address for /healthz and /metrics (METRICS_ADDR).
	MetricsAddr string
	// DefaultTTL is applied to endpoints without an explicit TTL (DEFAULT_TTL).
	DefaultTTL int64
	// LogLevel is the logrus level name (LOG_LEVEL).
	LogLevel string
}

// Load reads configuration from the environment and returns an error if
// required values are missing or invalid.
func Load() (Config, error) {
	cfg := Config{
		PATFile:     strings.TrimSpace(os.Getenv("NETBIRD_PAT_FILE")),
		BaseURL:     envOr("NETBIRD_BASE_URL", defaultBaseURL),
		WebhookAddr: envOr("WEBHOOK_ADDR", defaultWebhookAddr),
		MetricsAddr: envOr("METRICS_ADDR", defaultMetricsAddr),
		LogLevel:    envOr("LOG_LEVEL", defaultLogLevel),
		DefaultTTL:  defaultTTL,
	}
	if cfg.PATFile == "" {
		return Config{}, fmt.Errorf("config: NETBIRD_PAT_FILE must be set to a file holding the NetBird personal access token")
	}
	pat, err := os.ReadFile(cfg.PATFile)
	if err != nil {
		return Config{}, fmt.Errorf("config: read PAT file %q: %w", cfg.PATFile, err)
	}
	cfg.PAT = strings.TrimSpace(string(pat))
	if cfg.PAT == "" {
		return Config{}, fmt.Errorf("config: PAT file %q is empty", cfg.PATFile)
	}
	if raw := strings.TrimSpace(os.Getenv("DOMAIN_FILTER")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				cfg.DomainFilter = append(cfg.DomainFilter, part)
			}
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DEFAULT_TTL")); raw != "" {
		ttl, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ttl < 0 {
			return Config{}, fmt.Errorf("config: DEFAULT_TTL must be a non-negative integer, got %q", raw)
		}
		cfg.DefaultTTL = ttl
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
