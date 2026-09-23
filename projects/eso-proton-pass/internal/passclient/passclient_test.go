package passclient

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBoundaryNotFoundClassification checks the ONLY substring match in the
// codebase: CLI stderr classified once at the boundary into ErrNotFound,
// matched above with errors.Is.
func TestBoundaryNotFoundClassification(t *testing.T) {
	for _, stderr := range []string{
		"item not found",
		"Error 404 from backend",
		"no such vault",
	} {
		if !isNotFoundOutput(stderr) {
			t.Errorf("stderr %q: expected not-found classification", stderr)
		}
		if !isNotFoundOutput("prefix " + stderr + " suffix") {
			t.Errorf("stderr %q: expected wrapped classification", stderr)
		}
	}
	for _, stderr := range []string{"", "connection reset", "permission denied"} {
		if isNotFoundOutput(stderr) {
			t.Errorf("stderr %q: must not classify as not-found", stderr)
		}
	}
}

// TestExecErrorRedactsStderr checks ExecError.Error carries only the
// subcommand: stderr (secret-adjacent) is reachable via the struct for
// operator debugging but never in the message string.
func TestExecErrorRedactsStderr(t *testing.T) {
	err := &ExecError{Op: "inject", Stderr: "token=supersecret path=/etc/x", Err: errors.New("exit status 1")}
	if strings.Contains(err.Error(), "supersecret") || strings.Contains(err.Error(), "/etc/x") {
		t.Errorf("ExecError leaks stderr: %q", err.Error())
	}
	if !errors.Is(err, err) {
		t.Error("ExecError must be matchable in a chain")
	}
	var execErr *ExecError
	wrapped := errors.Join(errors.New("layer"), err)
	if !errors.As(wrapped, &execErr) {
		t.Fatal("errors.As must find *ExecError through wrapping")
	}
	if execErr.Stderr == "" {
		t.Error("ExecError must retain stderr for operator debugging")
	}
}

// TestDummySecretNeverLogged is the negative secret-safety test: a dummy
// secret resolved through a stubbed binary must never appear in log output.
func TestDummySecretNeverLogged(t *testing.T) {
	const dummySecret = "dummy-secret-value-abcdef123456"
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.sh")
	script := "#!/bin/sh\nprintf '%s' '" + dummySecret + "'\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	c := New(Options{
		BinaryPath: stub,
		Logger:     slog.New(slog.NewTextHandler(&logs, nil)),
	})
	out, err := c.ResolveSecret(t.Context(), "pass://vault/item/field", "test")
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if out != dummySecret {
		t.Fatalf("got %q, want dummy secret", out)
	}
	if strings.Contains(logs.String(), dummySecret) {
		t.Errorf("log output contains dummy secret:\n%s", logs.String())
	}
}

func TestReadPATFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pat")
	if err := os.WriteFile(p, []byte("pst_test::key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPATFile(p)
	if err != nil {
		t.Fatalf("ReadPATFile: %v", err)
	}
	if got != "pst_test::key" {
		t.Errorf("got %q, want trimmed token", got)
	}
}

func TestReadPATFileErrors(t *testing.T) {
	if _, err := ReadPATFile(""); err == nil {
		t.Error("expected error for empty path")
	}
	if _, err := ReadPATFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("expected error for missing file")
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPATFile(empty); err == nil {
		t.Error("expected error for empty file")
	}
}

func TestNewAgentReasonUniqueAndBounded(t *testing.T) {
	a, b := NewAgentReason("eso-proton-pass-get"), NewAgentReason("eso-proton-pass-get")
	if a == b {
		t.Error("expected unique reasons per call")
	}
	if !strings.HasPrefix(a, "eso-proton-pass-get-exec-") {
		t.Errorf("unexpected reason format %q", a)
	}
	long := NewAgentReason(strings.Repeat("x", 500))
	if len(long) > reasonMaxLen {
		t.Errorf("reason length %d exceeds %d", len(long), reasonMaxLen)
	}
}

func TestBaseEnvDisablesTelemetry(t *testing.T) {
	c := New(Options{})
	env := c.baseEnv()
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"PROTON_PASS_DISABLE_TELEMETRY=1",
		"PASS_LOG_LEVEL=off",
		"PROTON_PASS_KEY_PROVIDER=fs",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("baseEnv missing %q (got %q)", want, joined)
		}
	}
	for _, e := range env {
		if strings.HasPrefix(e, "PROTON_PASS_PERSONAL_ACCESS_TOKEN=") {
			t.Error("baseEnv must not carry the PAT")
		}
	}
}
