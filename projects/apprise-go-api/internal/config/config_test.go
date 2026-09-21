package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("APPRISE_STATEFUL_MODE", "")
	t.Setenv("APPRISE_STATELESS_STORAGE", "")
	t.Setenv("SECRET_KEY_FILE", "")
	t.Setenv("SECRET_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.StatefulMode != "disabled" {
		t.Errorf("StatefulMode = %q, want %q", cfg.StatefulMode, "disabled")
	}
	if cfg.StatelessStorage != "no" {
		t.Errorf("StatelessStorage = %q, want %q", cfg.StatelessStorage, "no")
	}
	if cfg.RecursionMax != 1 {
		t.Errorf("RecursionMax = %d, want 1", cfg.RecursionMax)
	}
	if cfg.WebhookMappingMaxDepth != 5 {
		t.Errorf("WebhookMappingMaxDepth = %d, want 5", cfg.WebhookMappingMaxDepth)
	}
	if cfg.AttachSizeMB != 200 {
		t.Errorf("AttachSizeMB = %d, want 200", cfg.AttachSizeMB)
	}
	if cfg.MaxAttachments != 6 {
		t.Errorf("MaxAttachments = %d, want 6", cfg.MaxAttachments)
	}
}

func TestLoadPortAndLogLevel(t *testing.T) {
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadRejectsStatefulMode(t *testing.T) {
	t.Setenv("APPRISE_STATEFUL_MODE", "enabled")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for stateful mode")
	}
}

func TestLoadRejectsPersistentStorage(t *testing.T) {
	t.Setenv("APPRISE_STATELESS_STORAGE", "yes")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for persistent storage")
	}
}

func TestLoadRejectsBadLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "bogus")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for bad LOG_LEVEL")
	}
}

func TestLoadRejectsNegativeRecursionMax(t *testing.T) {
	t.Setenv("APPRISE_RECURSION_MAX", "-1")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for negative recursion max")
	}
}

func TestLoadSecretKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	t.Setenv("SECRET_KEY_FILE", path)
	t.Setenv("SECRET_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.SecretKey != "s3cr3t" {
		t.Errorf("SecretKey = %q, want %q", cfg.SecretKey, "s3cr3t")
	}
}

func TestLoadMissingSecretKeyFile(t *testing.T) {
	t.Setenv("SECRET_KEY_FILE", filepath.Join(t.TempDir(), "does-not-exist"))

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for missing SECRET_KEY_FILE")
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("APPRISE_GO_API_TEST_ENVOR", "value")
	if got := envOr("APPRISE_GO_API_TEST_ENVOR", "fallback"); got != "value" {
		t.Errorf("envOr() = %q, want %q", got, "value")
	}
	t.Setenv("APPRISE_GO_API_TEST_ENVOR", "")
	if got := envOr("APPRISE_GO_API_TEST_ENVOR", "fallback"); got != "fallback" {
		t.Errorf("envOr() = %q, want %q", got, "fallback")
	}
}
