// Table tests for the notify error contract (see errors.go).
package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/attach"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

func TestStatusCodeOfTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil 200", err: nil, want: http.StatusOK},
		{name: "invalid tag 400", err: errInvalidTag, want: http.StatusBadRequest},
		{name: "wrapped invalid tag 400", err: fmt.Errorf("tag x: %w", errInvalidTag), want: http.StatusBadRequest},
		{name: "invalid format 400", err: errInvalidFormat, want: http.StatusBadRequest},
		{name: "invalid recursion 400", err: errInvalidRecursion, want: http.StatusBadRequest},
		{name: "remap failed 400", err: errRemapFailed, want: http.StatusBadRequest},
		{name: "wrapped remap failed 400", err: fmt.Errorf("apply: %w", errRemapFailed), want: http.StatusBadRequest},
		{name: "recursion limit 406", err: errRecursionLimit, want: http.StatusNotAcceptable},
		{name: "wrapped recursion limit 406", err: fmt.Errorf("count 2: %w", errRecursionLimit), want: http.StatusNotAcceptable},
		{name: "payload too large 431", err: errPayloadTooLarge, want: http.StatusRequestHeaderFieldsTooLarge},
		{name: "wrapped payload too large 431", err: fmt.Errorf("body: %w", errPayloadTooLarge), want: http.StatusRequestHeaderFieldsTooLarge},
		{name: "no targets 204", err: notify.ErrNoTargets, want: http.StatusNoContent},
		{name: "wrapped no targets 204", err: fmt.Errorf("filter: %w", notify.ErrNoTargets), want: http.StatusNoContent},
		{name: "bad attachment 400", err: attach.BadAttachment("bad entry"), want: http.StatusBadRequest},
		{name: "wrapped bad attachment 400", err: fmt.Errorf("stage: %w", attach.BadAttachment("bad entry")), want: http.StatusBadRequest},
		{name: "body too large 431", err: attach.BodyTooLarge(3 << 20), want: attach.StatusFieldsTooLarge},
		{name: "send failure 424", err: attach.WrapSendError("json://x", "a.txt", errors.New("boom")), want: http.StatusFailedDependency},
		{name: "wrapped send failure 424", err: fmt.Errorf("deliver: %w", attach.WrapSendError("json://x", "", errors.New("boom"))), want: http.StatusFailedDependency},
		{name: "delivery failure 500", err: errors.New("boom"), want: http.StatusInternalServerError},
		{name: "wrapped delivery failure 500", err: fmt.Errorf("send: %w", errors.New("boom")), want: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusCodeOf(tc.err); got != tc.want {
				t.Errorf("statusCodeOf(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestErrorMessagesLowercase asserts the canonical lowercase shape: sentinel
// and constructor messages start lowercase (never capitalized traces).
func TestErrorMessagesLowercase(t *testing.T) {
	errs := []error{
		errInvalidTag, errInvalidFormat, errInvalidRecursion, errRecursionLimit,
		errRemapFailed, errPayloadTooLarge, notify.ErrNoTargets,
		attach.BadAttachment("x"), attach.Denied("https://example.com/x.png"),
		attach.FetchFailed("https://example.com/x.png", errors.New("boom")),
		attach.BodyTooLarge(1), attach.Disabled(), attach.TooMany(7, 6),
		attach.TooLargeFile("a.txt", 1),
		attach.WrapSendError("json://x", "a.txt", errors.New("boom")),
	}
	for _, err := range errs {
		msg := err.Error()
		if msg == "" {
			t.Errorf("empty message for %T", err)
			continue
		}
		if first := msg[:1]; strings.ToLower(first) != first {
			t.Errorf("message %q starts uppercase, want lowercase", msg)
		}
	}
}

// TestDeniedFetchFailedRedactUserinfo asserts SSRF/ fetch errors never carry
// URL credentials: user:pass@ is stripped to ***@.
func TestDeniedFetchFailedRedactUserinfo(t *testing.T) {
	for _, err := range []error{
		attach.Denied("https://user:s3cr3t@example.com/file.png"),
		attach.FetchFailed("https://user:s3cr3t@example.com/file.png", errors.New("boom")),
	} {
		if strings.Contains(err.Error(), "s3cr3t") {
			t.Errorf("error carries credentials: %q", err.Error())
		}
		if !strings.Contains(err.Error(), "***@") {
			t.Errorf("error missing redaction marker: %q", err.Error())
		}
	}
}

// TestRedactCredentialsTable asserts the server-side redactor strips URL
// userinfo while leaving non-URL text untouched.
func TestRedactCredentialsTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{`send to https://user:pass@example.com/x failed`, `send to https://***@example.com/x failed`},
		{`send to json://localhost failed`, `send to json://localhost failed`},
		{`plain failure, no url`, `plain failure, no url`},
	}
	for _, tc := range cases {
		if got := redactCredentials(tc.in); got != tc.want {
			t.Errorf("redactCredentials(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFailureDetailsCarryNoCause asserts the 424 JSON details carry only the
// fixed message, never the raw send-error chain (no traces/SQL/PATs/paths).
func TestFailureDetailsCarryNoCause(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	h.fake.fail(errors.New("boom: secret-pat-123 /etc/passwd SELECT * FROM t"))
	rec := doPost(h, "/notify", "application/json",
		`{"urls":"json://localhost","body":"hi"}`,
		map[string]string{"Accept": "application/json"})
	if rec.Code != http.StatusFailedDependency {
		t.Fatalf("POST failing = %d, want 424", rec.Code)
	}
	body := rec.Body.String()
	for _, leak := range []string{"boom", "secret-pat-123", "/etc/passwd", "SELECT"} {
		if strings.Contains(body, leak) {
			t.Errorf("424 body leaks %q: %s", leak, body)
		}
	}
}
