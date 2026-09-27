// Package config loads the runtime configuration from the environment
// (stateless-only; secrets via *_FILE, never env values directly).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultAddr                = ":8080"
	defaultLogLevel            = "info"
	defaultRecursionMax        = 1
	defaultMappingMaxDepth     = 5
	defaultAttachDir           = ""
	defaultAttachSizeMB        = 200
	defaultMaxAttachments      = 6
	defaultUploadMaxMemorySize = 3
	defaultStatefulMode        = "disabled"
	defaultStatelessStorage    = "no"
	defaultInterpretEmojis     = false
	defaultHTTPRedirects       = true
	defaultCallTimeoutSecs     = 30
	defaultShutdownTimeoutSecs = 10
	maxHeaderBytes             = 1 << 20 // 1 MiB
	maxUploadMemoryBytes       = 32 << 20
)

// Config is the runtime configuration of apprise-go-api.
type Config struct {
	// Addr is the listen address for the HTTP API (HTTP_PORT derived, APPRISE_BASE_URL Host part ignored).
	Addr string
	// LogLevel is the slog level name (LOG_LEVEL).
	LogLevel string
	// Debug enables debug logging (DEBUG).
	Debug bool
	// BaseURL is the public base URL of the service (APPRISE_BASE_URL).
	BaseURL string
	// AllowedHosts is accepted but unenforced: no Host-header check exists
	// (ALLOWED_HOSTS). The Gateway fronts user traffic; like PLUGIN_PATHS
	// below, the variable is read for forward-compat but changes nothing.
	AllowedHosts []string
	// SecretKey authenticates internal callbacks, read only from SECRET_KEY_FILE.
	SecretKey string
	// SecretKeyFile is the path to the file holding the secret key (SECRET_KEY_FILE).
	SecretKeyFile string
	// Timezone for log timestamps (TZ).
	Timezone string
	// PUID/PGID record the desired runtime ownership (informational under distroless nonroot).
	PUID int
	PGID int
	// WorkerCount is accepted but unenforced: delivery is sequential per
	// call (WORKER_COUNT, 0 = GOMAXPROCS default). Like PLUGIN_PATHS below,
	// the variable is read for forward-compat but changes nothing.
	WorkerCount int
	// CallTimeoutSecs bounds a single notify call (TIMEOUT).
	CallTimeoutSecs int
	// ShutdownTimeoutSecs bounds graceful shutdown (default 10).
	ShutdownTimeoutSecs int

	// StatefulMode is always "disabled" (APPRISE_STATEFUL_MODE); any other value is rejected.
	StatefulMode string
	// StatelessURLs is the fallback URL set when a request carries none (APPRISE_STATELESS_URLS).
	StatelessURLs string
	// StatelessStorage is always "no" (APPRISE_STATELESS_STORAGE); persistence is unsupported.
	StatelessStorage string

	// AttachDir stages request-scoped attachment temp files (APPRISE_ATTACH_DIR).
	AttachDir string
	// AttachSizeMB is the per-file attachment limit in MiB (APPRISE_ATTACH_SIZE, <=0 disables attachments).
	AttachSizeMB int64
	// MaxAttachments caps attachments per request (APPRISE_MAX_ATTACHMENTS, 0 = unlimited).
	MaxAttachments int
	// UploadMaxMemorySizeMB bounds in-memory multipart parsing in MiB (APPRISE_UPLOAD_MAX_MEMORY_SIZE).
	UploadMaxMemorySizeMB int64
	// AttachAllowURL is the allowlist for remote attachment URLs (APPRISE_ATTACH_ALLOW_URL).
	AttachAllowURL string
	// AttachRejectURL is the denylist for remote attachment URLs (APPRISE_ATTACH_REJECT_URL).
	AttachRejectURL string
	// AttachRejectSet reports whether APPRISE_ATTACH_REJECT_URL was
	// explicitly set (empty disables denials; unset applies the default).
	AttachRejectSet bool

	// WebhookMappingMaxDepth caps ':' remap lookup depth (APPRISE_WEBHOOK_MAPPING_MAX_DEPTH).
	WebhookMappingMaxDepth int
	// WebhookURL receives outbound result callbacks (APPRISE_WEBHOOK_URL).
	WebhookURL string
	// PluginPaths is accepted but unsupported: apprise-go has no dynamic plugin loading (APPRISE_PLUGIN_PATHS).
	PluginPaths string

	// DenyServices blocks notification services by name/prefix (APPRISE_DENY_SERVICES).
	DenyServices []string
	// AllowServices restricts notification services when non-empty (APPRISE_ALLOW_SERVICES, wins over deny).
	AllowServices []string
	// RecursionMax caps X-Apprise-Recursion-Count (APPRISE_RECURSION_MAX).
	RecursionMax int
	// InterpretEmojis is accepted but unenforced: apprise-go exposes no
	// emoji-expansion option (APPRISE_INTERPRET_EMOJIS). Like PLUGIN_PATHS,
	// the variable is read for forward-compat but changes nothing.
	InterpretEmojis bool
	// HTTPRedirects is accepted but unenforced: apprise-go exposes no
	// redirect-policy option (APPRISE_HTTP_REDIRECTS). Like PLUGIN_PATHS,
	// the variable is read for forward-compat but changes nothing.
	HTTPRedirects bool
}

// Load reads configuration from the environment and returns an error if
// required values are missing or invalid.
func Load() (Config, error) {
	strict := func(set func(int), key string, fallback int) error {
		v, err := envInt(key, fallback)
		if err != nil {
			return err
		}
		set(v)
		return nil
	}
	strict64 := func(set func(int64), key string, fallback int64) error {
		v, err := envInt64(key, fallback)
		if err != nil {
			return err
		}
		set(v)
		return nil
	}
	strictBool := func(set func(bool), key string, fallback bool) error {
		v, err := envBool(key, fallback)
		if err != nil {
			return err
		}
		set(v)
		return nil
	}
	cfg := Config{
		Addr:                addrFromPort(envOr("HTTP_PORT", "8080")),
		LogLevel:            strings.ToLower(envOr("LOG_LEVEL", defaultLogLevel)),
		BaseURL:             envOr("APPRISE_BASE_URL", ""),
		AllowedHosts:        envCSV("ALLOWED_HOSTS"),
		SecretKeyFile:       strings.TrimSpace(os.Getenv("SECRET_KEY_FILE")),
		Timezone:            envOr("TZ", "UTC"),
		ShutdownTimeoutSecs: defaultShutdownTimeoutSecs,

		StatefulMode:     envOr("APPRISE_STATEFUL_MODE", defaultStatefulMode),
		StatelessURLs:    strings.TrimSpace(os.Getenv("APPRISE_STATELESS_URLS")),
		StatelessStorage: envOr("APPRISE_STATELESS_STORAGE", defaultStatelessStorage),
		AttachDir:        envOr("APPRISE_ATTACH_DIR", defaultAttachDir),
		AttachAllowURL:   strings.TrimSpace(os.Getenv("APPRISE_ATTACH_ALLOW_URL")),
		AttachRejectURL:  strings.TrimSpace(os.Getenv("APPRISE_ATTACH_REJECT_URL")),
		AttachRejectSet:  envSet("APPRISE_ATTACH_REJECT_URL"),
		WebhookURL:       strings.TrimSpace(os.Getenv("APPRISE_WEBHOOK_URL")),
		PluginPaths:      strings.TrimSpace(os.Getenv("APPRISE_PLUGIN_PATHS")),

		DenyServices:  envCSV("APPRISE_DENY_SERVICES"),
		AllowServices: envCSV("APPRISE_ALLOW_SERVICES"),
	}
	// Strict numerics/bools: non-empty unparseable values fail startup
	// instead of silently falling back (PUID/PGID already behave this way).
	for _, step := range []func() error{
		func() error { return strictBool(func(v bool) { cfg.Debug = v }, "DEBUG", false) },
		func() error { return strict(func(v int) { cfg.WorkerCount = v }, "WORKER_COUNT", 0) },
		func() error {
			return strict(func(v int) { cfg.CallTimeoutSecs = v }, "TIMEOUT", defaultCallTimeoutSecs)
		},
		func() error {
			return strict64(func(v int64) { cfg.AttachSizeMB = v }, "APPRISE_ATTACH_SIZE", defaultAttachSizeMB)
		},
		func() error {
			return strict(func(v int) { cfg.MaxAttachments = v }, "APPRISE_MAX_ATTACHMENTS", defaultMaxAttachments)
		},
		func() error {
			return strict64(func(v int64) { cfg.UploadMaxMemorySizeMB = v }, "APPRISE_UPLOAD_MAX_MEMORY_SIZE", defaultUploadMaxMemorySize)
		},
		func() error {
			return strict(func(v int) { cfg.WebhookMappingMaxDepth = v }, "APPRISE_WEBHOOK_MAPPING_MAX_DEPTH", defaultMappingMaxDepth)
		},
		func() error {
			return strict(func(v int) { cfg.RecursionMax = v }, "APPRISE_RECURSION_MAX", defaultRecursionMax)
		},
		func() error {
			return strictBool(func(v bool) { cfg.InterpretEmojis = v }, "APPRISE_INTERPRET_EMOJIS", defaultInterpretEmojis)
		},
		func() error {
			return strictBool(func(v bool) { cfg.HTTPRedirects = v }, "APPRISE_HTTP_REDIRECTS", defaultHTTPRedirects)
		},
	} {
		if err := step(); err != nil {
			return Config{}, err
		}
	}

	if raw := strings.TrimSpace(os.Getenv("PUID")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return Config{}, fmt.Errorf("config: PUID must be a non-negative integer, got %q", raw)
		}
		cfg.PUID = v
	}
	if raw := strings.TrimSpace(os.Getenv("PGID")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return Config{}, fmt.Errorf("config: PGID must be a non-negative integer, got %q", raw)
		}
		cfg.PGID = v
	}
	if cfg.SecretKeyFile != "" {
		secret, err := os.ReadFile(filepath.Clean(cfg.SecretKeyFile))
		if err != nil {
			return Config{}, fmt.Errorf("config: read SECRET_KEY_FILE %q: %w", cfg.SecretKeyFile, err)
		}
		cfg.SecretKey = strings.TrimSpace(string(secret))
	}
	if !validLogLevel(cfg.LogLevel) {
		return Config{}, fmt.Errorf("config: LOG_LEVEL must be one of debug, info, warn, error, got %q", cfg.LogLevel)
	}
	if !strings.EqualFold(cfg.StatefulMode, defaultStatefulMode) {
		return Config{}, fmt.Errorf("config: APPRISE_STATEFUL_MODE must be %q (stateful endpoints are unsupported), got %q", defaultStatefulMode, cfg.StatefulMode)
	}
	if !strings.EqualFold(cfg.StatelessStorage, defaultStatelessStorage) {
		return Config{}, fmt.Errorf("config: APPRISE_STATELESS_STORAGE must be %q (persistent storage is unsupported), got %q", defaultStatelessStorage, cfg.StatelessStorage)
	}
	cfg.StatelessStorage = defaultStatelessStorage
	if cfg.RecursionMax < 0 {
		return Config{}, fmt.Errorf("config: APPRISE_RECURSION_MAX must be a non-negative integer, got %d", cfg.RecursionMax)
	}
	if cfg.WebhookMappingMaxDepth <= 0 {
		return Config{}, fmt.Errorf("config: APPRISE_WEBHOOK_MAPPING_MAX_DEPTH must be a positive integer, got %d", cfg.WebhookMappingMaxDepth)
	}
	if cfg.CallTimeoutSecs <= 0 {
		return Config{}, fmt.Errorf("config: TIMEOUT must be a positive integer of seconds, got %d", cfg.CallTimeoutSecs)
	}
	if cfg.WorkerCount < 0 {
		return Config{}, fmt.Errorf("config: WORKER_COUNT must be a non-negative integer, got %d", cfg.WorkerCount)
	}
	return cfg, nil
}

// MaxHeaderBytes caps HTTP request header size at 1 MiB.
func MaxHeaderBytes() int { return maxHeaderBytes }

// MaxUploadMemoryBytes caps in-memory multipart buffering before spilling to disk.
func MaxUploadMemoryBytes() int64 { return maxUploadMemoryBytes }

// AttachSizeBytes returns the per-file attachment limit in bytes
// (APPRISE_ATTACH_SIZE in MiB). A non-positive result disables attachments.
func (c Config) AttachSizeBytes() int64 { return c.AttachSizeMB * 1024 * 1024 }

// UploadMaxMemoryBytes returns the multipart/JSON body budget in bytes
// (APPRISE_UPLOAD_MAX_MEMORY_SIZE in MiB, negative values use magnitude).
func (c Config) UploadMaxMemoryBytes() int64 {
	if c.UploadMaxMemorySizeMB < 0 {
		return -c.UploadMaxMemorySizeMB * 1024 * 1024
	}
	return c.UploadMaxMemorySizeMB * 1024 * 1024
}

// DefaultAttachAllowURL is the APPRISE_ATTACH_ALLOW_URL default.
const DefaultAttachAllowURL = "*"

// DefaultAttachRejectURL is the out-of-box APPRISE_ATTACH_REJECT_URL
// default (unset env means this; empty env disables denials). The
// `internal` token makes the default fail closed: every attachment host
// is DNS-resolved and IP-classified (loopback, private, link-local,
// reserved, multicast, CGN, alternate encodings blocked; unresolvable
// hosts blocked), so ALLOW=* out of the box can no longer reach the
// instance metadata service or the cluster network.
const DefaultAttachRejectURL = "127.0.* localhost* internal"

// AttachAllowURLOrDefault returns the configured SSRF allowlist or "*".
func (c Config) AttachAllowURLOrDefault() string {
	if c.AttachAllowURL == "" {
		return DefaultAttachAllowURL
	}
	return c.AttachAllowURL
}

// AttachRejectURLOrDefault returns the configured SSRF denylist. An
// explicitly empty APPRISE_ATTACH_REJECT_URL disables denials; when the
// variable is unset the Python-parity default applies.
func (c Config) AttachRejectURLOrDefault() string {
	if !c.AttachRejectSet {
		return DefaultAttachRejectURL
	}
	return c.AttachRejectURL
}

func addrFromPort(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return defaultAddr
	}
	port = strings.TrimPrefix(port, ":")
	return ":" + port
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// envSet reports whether key is present in the environment (even when its
// value is empty), so callers can tell "unset" apart from "set empty".
func envSet(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func envCSV(key string) []string {
	var out []string
	for _, part := range strings.Split(os.Getenv(key), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// envInt reads key as an integer, failing fast on a non-empty unparseable
// value (empty means the fallback). Silent fallbacks would hide typos, so
// every numeric/bool knob parses strictly like PUID/PGID below.
func envInt(key string, fallback int) (int, error) {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("config: %s must be an integer, got %q", key, raw)
		}
		return v, nil
	}
	return fallback, nil
}

func envInt64(key string, fallback int64) (int64, error) {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("config: %s must be an integer, got %q", key, raw)
		}
		return v, nil
	}
	return fallback, nil
}

func envBool(key string, fallback bool) (bool, error) {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		v, err := strconv.ParseBool(strings.ToLower(raw))
		if err != nil {
			return false, fmt.Errorf("config: %s must be a boolean, got %q", key, raw)
		}
		return v, nil
	}
	return fallback, nil
}

func validLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "warning", "error":
		return true
	default:
		return false
	}
}
