package config

import (
	"testing"
	"time"
)

func TestLoadRequiresPATFile(t *testing.T) {
	t.Setenv("PROTON_PASS_PAT_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when PROTON_PASS_PAT_FILE is unset")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PROTON_PASS_PAT_FILE", "/run/secrets/pat")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("PASS_CLI_BIN", "")
	t.Setenv("PASS_CLI_TIMEOUT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != DefaultListenAddr {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, DefaultListenAddr)
	}
	if cfg.PassCLIBinary != DefaultPassCLIBin {
		t.Errorf("PassCLIBinary = %q, want %q", cfg.PassCLIBinary, DefaultPassCLIBin)
	}
	if cfg.ExecTimeout != DefaultTimeout {
		t.Errorf("ExecTimeout = %v, want %v", cfg.ExecTimeout, DefaultTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("PROTON_PASS_PAT_FILE", "/run/secrets/pat")
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("PASS_CLI_BIN", "/usr/local/bin/pass-cli")
	t.Setenv("PASS_CLI_TIMEOUT", "30s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":9090" || cfg.PassCLIBinary != "/usr/local/bin/pass-cli" || cfg.ExecTimeout != 30*time.Second {
		t.Errorf("unexpected overrides: %+v", cfg)
	}
}

func TestLoadBadTimeout(t *testing.T) {
	t.Setenv("PROTON_PASS_PAT_FILE", "/run/secrets/pat")
	for _, v := range []string{"bogus", "-5s", "0s"} {
		t.Setenv("PASS_CLI_TIMEOUT", v)
		if _, err := Load(); err == nil {
			t.Errorf("expected error for PASS_CLI_TIMEOUT=%q", v)
		}
	}
}
