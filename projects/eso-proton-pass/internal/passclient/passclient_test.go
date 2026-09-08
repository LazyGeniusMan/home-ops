package passclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
