// Package config loads the provider configuration from the environment
// (PAT from PROTON_PASS_PAT_FILE, never an env value directly).
package config

import (
	"fmt"
	"os"
	"time"
)

// Defaults for optional settings.
const (
	DefaultListenAddr = ":8080"
	DefaultPassCLIBin = "pass-cli"
	DefaultTimeout    = 60 * time.Second
)

// Config is the validated runtime configuration.
type Config struct {
	// PATFile is the path to the file holding the Proton Pass PAT.
	PATFile string
	// ListenAddr is the HTTP listen address, e.g. ":8080".
	ListenAddr string
	// PassCLIBinary is the path to the pass-cli executable.
	PassCLIBinary string
	// SessionDir overrides PROTON_PASS_SESSION_DIR when non-empty.
	SessionDir string
	// ExecTimeout bounds every pass-cli invocation.
	ExecTimeout time.Duration
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		PATFile:       os.Getenv("PROTON_PASS_PAT_FILE"),
		ListenAddr:    envOr("LISTEN_ADDR", DefaultListenAddr),
		PassCLIBinary: envOr("PASS_CLI_BIN", DefaultPassCLIBin),
		SessionDir:    os.Getenv("PROTON_PASS_SESSION_DIR"),
		ExecTimeout:   DefaultTimeout,
	}
	if v := os.Getenv("PASS_CLI_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PASS_CLI_TIMEOUT %q: %w", v, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("invalid PASS_CLI_TIMEOUT %q: must be positive", v)
		}
		cfg.ExecTimeout = d
	}
	if cfg.PATFile == "" {
		return Config{}, fmt.Errorf("missing required env PROTON_PASS_PAT_FILE: set it to a file holding the Proton Pass PAT")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
