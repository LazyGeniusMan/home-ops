// Package passclient resolves Proton Pass secrets via the pass-cli binary
// (`pass-cli inject` over stdin; PAT from file via child-process env;
// fresh agent reason per exec; telemetry off on every invocation).
package passclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNotFound marks CLI-reported missing secrets (%w-wrapped; provider
// maps it to HTTP 404).
var ErrNotFound = errors.New("pass-cli: secret not found")

// ExecError is a pass-cli invocation failure. Stderr is retained for
// operator logs only (never in Error), so secrets cannot leak into envelopes.
type ExecError struct {
	// Op is the subcommand, e.g. "inject".
	Op string
	// Stderr is the trimmed tail of CLI stderr (debugging only).
	Stderr string
	// Err is the exit/context cause.
	Err error
}

// Error implements error. It carries only the subcommand: no stderr, no
// paths, no tokens. Messages stay lowercase with no punctuation per the
// error contract (see internal/server/errors.go).
func (e *ExecError) Error() string {
	if e == nil {
		return "pass-cli: <nil>"
	}
	return "pass-cli " + e.Op + " failed"
}

// Unwrap returns the exit/context cause.
func (e *ExecError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

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

// run executes pass-cli, returning trimmed stdout. Stderr is logged once
// here; callers wrap with %w (never %v), lowercase, so secrets never leak.
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
		// Log stderr for operators; keep it out of the returned error
		// string (ExecError.Error carries only the subcommand).
		stderrTail := tail(stderr.String(), 500)
		c.logger.Error("pass-cli exec failed",
			slog.String("args", strings.Join(args, " ")),
			slog.String("stderr_tail", stderrTail),
		)
		return "", &ExecError{Op: firstArg(args), Stderr: stderrTail, Err: err}
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

// ResolveSecret resolves a pass:// URI via `pass-cli inject` (fresh audit
// reason per call; URIs redacted to vault/item/<field> in errors).
func (c *Client) ResolveSecret(ctx context.Context, uri, reasonPrefix string) (string, error) {
	if !strings.HasPrefix(uri, "pass://") {
		return "", fmt.Errorf("resolve: invalid uri: must start with pass://")
	}
	template := "{{ " + uri + " }}"
	out, err := c.run(ctx, []string{"inject"},
		template,
		[]string{"PROTON_PASS_AGENT_REASON=" + NewAgentReason(reasonPrefix)},
	)
	if err != nil {
		var execErr *ExecError
		if errors.As(err, &execErr) && isNotFoundOutput(execErr.Stderr) {
			return "", fmt.Errorf("resolve %s: %w", redactURI(uri), ErrNotFound)
		}
		return "", fmt.Errorf("resolve %s: %w", redactURI(uri), err)
	}
	return out, nil
}

// Ping checks pass-cli reachability without touching secrets: it runs a
// metadata-only subcommand whose stdout carries no secret material. The
// caller (readiness probe) cares only about success vs failure.
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.run(ctx, []string{"info"}, "", nil); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}

// isNotFoundOutput classifies CLI stderr at the boundary. This is the ONLY
// substring match in the codebase: the CLI exposes no typed errors, so the
// boundary converts its free-text stderr into the ErrNotFound sentinel once,
// and every layer above matches with errors.Is.
func isNotFoundOutput(stderr string) bool {
	msg := strings.ToLower(stderr)
	for _, marker := range []string{"not found", "no such", "does not exist", "not exist", "404"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
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
