package config

import (
	"os"
	"testing"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/attach"
)

func TestAttachSizeBytes(t *testing.T) {
	cfg := Config{AttachSizeMB: 200}
	if got := cfg.AttachSizeBytes(); got != 200*1024*1024 {
		t.Errorf("AttachSizeBytes() = %d, want %d", got, 200*1024*1024)
	}
	if got := (Config{AttachSizeMB: 0}).AttachSizeBytes(); got != 0 {
		t.Errorf("AttachSizeBytes(disabled) = %d, want 0", got)
	}
}

func TestUploadMaxMemoryBytes(t *testing.T) {
	cfg := Config{UploadMaxMemorySizeMB: 3}
	if got := cfg.UploadMaxMemoryBytes(); got != 3*1024*1024 {
		t.Errorf("UploadMaxMemoryBytes() = %d, want %d", got, 3*1024*1024)
	}
}

func TestAttachPolicyDefaults(t *testing.T) {
	t.Setenv("APPRISE_ATTACH_ALLOW_URL", "")
	if err := os.Unsetenv("APPRISE_ATTACH_REJECT_URL"); err != nil {
		t.Fatalf("Unsetenv() = %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got := cfg.AttachAllowURLOrDefault(); got != "*" {
		t.Errorf("AttachAllowURLOrDefault() = %q, want *", got)
	}
	if cfg.AttachRejectURLOrDefault() != DefaultAttachRejectURL {
		t.Errorf("AttachRejectURLOrDefault() = %q, want default %q", cfg.AttachRejectURLOrDefault(), DefaultAttachRejectURL)
	}
	if DefaultAttachRejectURL != "127.0.* localhost* internal" {
		t.Errorf("DefaultAttachRejectURL = %q, want fail-closed default", DefaultAttachRejectURL)
	}
	// Explicitly empty still opts out of denials.
	t.Setenv("APPRISE_ATTACH_REJECT_URL", "")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.AttachRejectURLOrDefault() != "" {
		t.Errorf("AttachRejectURLOrDefault(explicit empty) = %q, want empty (denials off)", cfg.AttachRejectURLOrDefault())
	}
}

// TestAttachPolicyDefaultDeniesInternal verifies the M-S1 fail-closed
// default: an unset APPRISE_ATTACH_REJECT_URL still rejects loopback,
// link-local metadata (169.254.169.254), and RFC-1918 targets via the
// `internal` classifier (IP literals classify without DNS; the
// localhost* prefix rule covers the hostname without resolving).
func TestAttachPolicyDefaultDeniesInternal(t *testing.T) {
	t.Setenv("APPRISE_ATTACH_ALLOW_URL", "")
	if err := os.Unsetenv("APPRISE_ATTACH_REJECT_URL"); err != nil {
		t.Fatalf("Unsetenv() = %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	pol := attach.NewPolicy(cfg.AttachAllowURLOrDefault(), cfg.AttachRejectURLOrDefault())
	for _, raw := range []string{
		"http://127.0.0.1/x",
		"http://localhost/x",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/x",
		"http://192.168.1.1/x",
	} {
		if pol.IsAllowed(raw) {
			t.Errorf("default policy IsAllowed(%q) = true, want false", raw)
		}
	}
	if !pol.IsAllowed("http://example.com/x") {
		t.Error("default policy IsAllowed(public host) = false, want true")
	}
}

func TestAttachPolicyConfigured(t *testing.T) {
	t.Setenv("APPRISE_ATTACH_ALLOW_URL", "example.com")
	t.Setenv("APPRISE_ATTACH_REJECT_URL", "internal")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got := cfg.AttachAllowURLOrDefault(); got != "example.com" {
		t.Errorf("AttachAllowURLOrDefault() = %q, want example.com", got)
	}
	if got := cfg.AttachRejectURLOrDefault(); got != "internal" {
		t.Errorf("AttachRejectURLOrDefault() = %q, want internal", got)
	}
}
