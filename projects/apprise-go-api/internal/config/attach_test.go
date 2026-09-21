package config

import (
	"testing"
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
	t.Setenv("APPRISE_ATTACH_REJECT_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got := cfg.AttachAllowURLOrDefault(); got != "*" {
		t.Errorf("AttachAllowURLOrDefault() = %q, want *", got)
	}
	if cfg.AttachRejectURLOrDefault() != "" {
		t.Errorf("AttachRejectURLOrDefault() = %q, want empty (denials off when unset)", cfg.AttachRejectURLOrDefault())
	}
	if DefaultAttachRejectURL != "127.0.* localhost*" {
		t.Errorf("DefaultAttachRejectURL = %q, want Python default", DefaultAttachRejectURL)
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
