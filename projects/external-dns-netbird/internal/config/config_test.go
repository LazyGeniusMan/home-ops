package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writePAT(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pat")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"NETBIRD_PAT_FILE", "NETBIRD_BASE_URL", "DOMAIN_FILTER", "WEBHOOK_ADDR", "METRICS_ADDR", "DEFAULT_TTL", "LOG_LEVEL"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("NETBIRD_PAT_FILE", writePAT(t, "secret-pat\n"))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PAT != "secret-pat" {
		t.Errorf("PAT not trimmed: %q", cfg.PAT)
	}
	if cfg.BaseURL != "https://api.netbird.io" {
		t.Errorf("unexpected base URL: %q", cfg.BaseURL)
	}
	if cfg.WebhookAddr != "127.0.0.1:8888" || cfg.MetricsAddr != ":8080" {
		t.Errorf("unexpected addrs: %+v", cfg)
	}
	if cfg.DefaultTTL != 300 || cfg.LogLevel != "info" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadMissingPATFile(t *testing.T) {
	clearEnv(t)
	if _, err := Load(); err == nil {
		t.Error("expected error when NETBIRD_PAT_FILE is unset")
	}
}

func TestLoadEmptyPATFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("NETBIRD_PAT_FILE", writePAT(t, "  \n"))
	if _, err := Load(); err == nil {
		t.Error("expected error for empty PAT file")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("NETBIRD_PAT_FILE", writePAT(t, "pat"))
	t.Setenv("DOMAIN_FILTER", "example.com, other.net")
	t.Setenv("DEFAULT_TTL", "60")
	t.Setenv("LOG_LEVEL", "debug")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.DomainFilter) != 2 || cfg.DomainFilter[0] != "example.com" {
		t.Errorf("unexpected filter: %v", cfg.DomainFilter)
	}
	if cfg.DefaultTTL != 60 || cfg.LogLevel != "debug" {
		t.Errorf("unexpected overrides: %+v", cfg)
	}
}

func TestLoadBadTTL(t *testing.T) {
	clearEnv(t)
	t.Setenv("NETBIRD_PAT_FILE", writePAT(t, "pat"))
	t.Setenv("DEFAULT_TTL", "-5")
	if _, err := Load(); err == nil {
		t.Error("expected error for negative DEFAULT_TTL")
	}
}
