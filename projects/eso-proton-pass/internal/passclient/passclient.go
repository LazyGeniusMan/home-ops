// Package passclient shells out to the pass-cli binary to resolve Proton Pass
// secrets.
//
// Design notes:
//
//   - The client never takes the PAT as an env value: it reads the token from
//     the PROTON_PASS_PAT_FILE file content at startup and passes it to
//     `pass-cli login` via the PROTON_PASS_PERSONAL_ACCESS_TOKEN env var on a
//     per-process environment.
//   - A fresh PROTON_PASS_AGENT_REASON is generated for every pass-cli exec so
//     agent audit logs attribute each read uniquely.
//   - Telemetry is disabled via PROTON_PASS_DISABLE_TELEMETRY=1 and
//     PASS_LOG_LEVEL=off on every invocation.
//   - Resolution uses `pass-cli inject` over stdin: the template
//     `{{ pass://vault/item/field }}` is resolved from the vault and the
//     substituted stdout is the secret value. URIs are never interpolated
//     into a shell command (no shell is used; exec argv only).
package passclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// reasonMaxLen is the pass-cli limit for PROTON_PASS_AGENT_REASON (300 chars).
const reasonMaxLen = 300

// Options configures a Client.
type Options struct {
	// BinaryPath is the pass-cli executable (default "pass-cli").
	BinaryPath string
	// SessionDir overrides PROTON_PASS_SESSION_DIR when non-empty.
	SessionDir string
	// Timeout bounds every pass-cli invocation (default 60s).
	Timeout time.Duration
	// Logger receives structured diagnostics (nil discards).
	Logger *slog.Logger
}

// Client resolves secrets via pass-cli.
type Client struct {
	binary     string
	sessionDir string
	timeout    time.Duration
	logger     *slog.Logger
}

// New builds a Client from Options.
func New(o Options) *Client {
	if o.BinaryPath == "" {
		o.BinaryPath = "pass-cli"
	}
	if o.Timeout <= 0 {
		o.Timeout = 60 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Client{
		binary:     o.BinaryPath,
		sessionDir: o.SessionDir,
		timeout:    o.Timeout,
		logger:     o.Logger,
	}
}

// ReadPATFile reads the PAT from the file at path, trimming trailing
// whitespace. The file must exist and be non-empty.
func ReadPATFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("PAT file path is empty (set PROTON_PASS_PAT_FILE)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read PAT file: %w", err)
	}
	pat := strings.TrimSpace(string(raw))
	if pat == "" {
		return "", fmt.Errorf("PAT file %q is empty", path)
	}
	return pat, nil
}

// NewAgentReason generates a fresh, unique PROTON_PASS_AGENT_REASON value for
// a single pass-cli invocation. The prefix identifies the caller for audit
// logs; the random suffix keeps every invocation distinct.
func NewAgentReason(prefix string) string {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return prefix + fmt.Sprintf("-%d", time.Now().UnixNano())
	}
	reason := prefix + "-exec-" + hex.EncodeToString(suffix[:])
	if len(reason) > reasonMaxLen {
		reason = reason[:reasonMaxLen]
	}
	return reason
}

// baseEnv returns the hardened environment applied to every pass-cli exec:
// telemetry off, CLI logs off, container-safe key/session storage.
func (c *Client) baseEnv() []string {
	env := []string{
		"PROTON_PASS_DISABLE_TELEMETRY=1",
		"PASS_LOG_LEVEL=off",
		// Containers cannot access the kernel keyring; use filesystem key
		// storage scoped to the session dir.
		"PROTON_PASS_KEY_PROVIDER=fs",
		"PROTON_PASS_LINUX_KEYRING=kernel",
	}
	if c.sessionDir != "" {
		env = append(env, "PROTON_PASS_SESSION_DIR="+c.sessionDir)
	}
	return env
}

// run executes pass-cli with args, stdin, and extra env entries. It returns
// trimmed stdout or a redacted error (stderr is logged, never returned, so
// secret material cannot leak through error strings).
func (c *Client) run(ctx context.Context, args []string, stdin string, extraEnv []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binary, args...)
	env := os.Environ()
	env = append(env, c.baseEnv()...)
	env = append(env, extraEnv...)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	c.logger.Debug("pass-cli exec",
		slog.String("args0", c.binary+" "+strings.Join(args, " ")),
		slog.Duration("elapsed", elapsed),
		slog.Bool("ok", err == nil),
	)
	if err != nil {
		// Log stderr for operators; keep it out of the returned error.
		c.logger.Error("pass-cli exec failed",
			slog.String("args", strings.Join(args, " ")),
			slog.String("stderr_tail", tail(stderr.String(), 500)),
		)
		return "", fmt.Errorf("pass-cli %s failed: %w", firstArg(args), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return "exec"
	}
	return args[0]
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// Login establishes a pass-cli session for the given PAT. The PAT travels
// only in the child process environment, never in argv or logs.
func (c *Client) Login(ctx context.Context, pat string) error {
	if strings.TrimSpace(pat) == "" {
		return fmt.Errorf("login: PAT is empty")
	}
	_, err := c.run(ctx, []string{"login"}, "",
		[]string{"PROTON_PASS_PERSONAL_ACCESS_TOKEN=" + pat})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

// ResolveSecret resolves a pass://vault/item/field URI to its secret value
// via `pass-cli inject`. A fresh audit reason is generated per call.
func (c *Client) ResolveSecret(ctx context.Context, uri, reasonPrefix string) (string, error) {
	if !strings.HasPrefix(uri, "pass://") {
		return "", fmt.Errorf("resolve: invalid URI %q: must start with pass://", uri)
	}
	template := "{{ " + uri + " }}"
	out, err := c.run(ctx, []string{"inject"},
		template,
		[]string{"PROTON_PASS_AGENT_REASON=" + NewAgentReason(reasonPrefix)},
	)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", redactURI(uri), err)
	}
	return out, nil
}

// redactURI keeps vault/item structure for diagnostics without exposing the
// field name (which can be sensitive, e.g. a custom secret label).
func redactURI(uri string) string {
	rest := strings.TrimPrefix(uri, "pass://")
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return "pass://<invalid>"
	}
	return "pass://" + parts[0] + "/" + parts[1] + "/<field>"
}
