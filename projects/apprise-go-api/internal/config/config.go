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
	// AllowedHosts limits Host header values (ALLOWED_HOSTS, comma-separated).
	AllowedHosts []string
	// SecretKey authenticates internal callbacks, read from SECRET_KEY_FILE (SECRET_KEY).
	SecretKey string
	// SecretKeyFile is the path to the file holding the secret key (SECRET_KEY_FILE).
	SecretKeyFile string
	// Timezone for log timestamps (TZ).
	Timezone string
	// PUID/PGID record the desired runtime ownership (informational under distroless nonroot).
	PUID int
	PGID int
	// WorkerCount bounds concurrent notify fan-out (WORKER_COUNT, 0 = GOMAXPROCS default).
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
	// InterpretEmojis enables emoji shortcode expansion (APPRISE_INTERPRET_EMOJIS).
	InterpretEmojis bool
	// HTTPRedirects enables following HTTP redirects (APPRISE_HTTP_REDIRECTS).
	HTTPRedirects bool
}

// Load reads configuration from the environment and returns an error if
// required values are missing or invalid.
func Load() (Config, error) {
	cfg := Config{
		Addr:                addrFromPort(envOr("HTTP_PORT", "8080")),
		LogLevel:            strings.ToLower(envOr("LOG_LEVEL", defaultLogLevel)),
		Debug:               envBool("DEBUG", false),
		BaseURL:             envOr("APPRISE_BASE_URL", ""),
		AllowedHosts:        envCSV("ALLOWED_HOSTS"),
		SecretKeyFile:       strings.TrimSpace(os.Getenv("SECRET_KEY_FILE")),
		Timezone:            envOr("TZ", "UTC"),
		WorkerCount:         envInt("WORKER_COUNT", 0),
		CallTimeoutSecs:     envInt("TIMEOUT", defaultCallTimeoutSecs),
		ShutdownTimeoutSecs: defaultShutdownTimeoutSecs,

		StatefulMode:           envOr("APPRISE_STATEFUL_MODE", defaultStatefulMode),
		StatelessURLs:          strings.TrimSpace(os.Getenv("APPRISE_STATELESS_URLS")),
		StatelessStorage:       envOr("APPRISE_STATELESS_STORAGE", defaultStatelessStorage),
		AttachDir:              envOr("APPRISE_ATTACH_DIR", defaultAttachDir),
		AttachSizeMB:           envInt64("APPRISE_ATTACH_SIZE", defaultAttachSizeMB),
		MaxAttachments:         envInt("APPRISE_MAX_ATTACHMENTS", defaultMaxAttachments),
		UploadMaxMemorySizeMB:  envInt64("APPRISE_UPLOAD_MAX_MEMORY_SIZE", defaultUploadMaxMemorySize),
		AttachAllowURL:         strings.TrimSpace(os.Getenv("APPRISE_ATTACH_ALLOW_URL")),
		AttachRejectURL:        strings.TrimSpace(os.Getenv("APPRISE_ATTACH_REJECT_URL")),
		WebhookMappingMaxDepth: envInt("APPRISE_WEBHOOK_MAPPING_MAX_DEPTH", defaultMappingMaxDepth),
		WebhookURL:             strings.TrimSpace(os.Getenv("APPRISE_WEBHOOK_URL")),
		PluginPaths:            strings.TrimSpace(os.Getenv("APPRISE_PLUGIN_PATHS")),

		DenyServices:    envCSV("APPRISE_DENY_SERVICES"),
		AllowServices:   envCSV("APPRISE_ALLOW_SERVICES"),
		RecursionMax:    envInt("APPRISE_RECURSION_MAX", defaultRecursionMax),
		InterpretEmojis: envBool("APPRISE_INTERPRET_EMOJIS", defaultInterpretEmojis),
		HTTPRedirects:   envBool("APPRISE_HTTP_REDIRECTS", defaultHTTPRedirects),
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
	} else if v := strings.TrimSpace(os.Getenv("SECRET_KEY")); v != "" {
		cfg.SecretKey = v
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
// (APPRISE_UPLOAD_MAX_MEMORY_SIZE in MiB). Python applies abs() to the env
// value; Load stores the raw value and this normalizes the sign so gates
// and tests share one conversion.
func (c Config) UploadMaxMemoryBytes() int64 {
	if c.UploadMaxMemorySizeMB < 0 {
		return -c.UploadMaxMemorySizeMB * 1024 * 1024
	}
	return c.UploadMaxMemorySizeMB * 1024 * 1024
}

// DefaultAttachAllowURL is Python's APPRISE_ATTACH_ALLOW_URL default.
const DefaultAttachAllowURL = "*"

// DefaultAttachRejectURL is Python's APPRISE_ATTACH_REJECT_URL default
// (APPRISE_ATTACH_REJECT_URL env, "127.0.* localhost*" when unset).
const DefaultAttachRejectURL = "127.0.* localhost*"

// AttachAllowURLOrDefault returns the configured SSRF allowlist or "*".
func (c Config) AttachAllowURLOrDefault() string {
	if c.AttachAllowURL == "" {
		return DefaultAttachAllowURL
	}
	return c.AttachAllowURL
}

// AttachRejectURLOrDefault returns the configured SSRF denylist. An empty
// configured value disables denials (no Python-style default is applied at
// load because the env may intentionally clear it); callers wanting the
// Python out-of-box default use DefaultAttachRejectURL.
func (c Config) AttachRejectURLOrDefault() string { return c.AttachRejectURL }

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

func envCSV(key string) []string {
	var out []string
	for _, part := range strings.Split(os.Getenv(key), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envInt(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			return v
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return v
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if v, err := strconv.ParseBool(strings.ToLower(raw)); err == nil {
			return v
		}
	}
	return fallback
}

func validLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "warning", "error":
		return true
	default:
		return false
	}
}
