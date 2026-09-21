// Package notify wraps the apprise-go notification engine with a
// request-scoped, timeout-bounded sender. Full G2 validation (type/format/
// tag grammar, URL parsing, recursion and allow/deny policy) lands in later
// goals; this file keeps the engine wiring thin.
package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	apprise "github.com/unraid/apprise-go"
)

// Request is the validated stateless notify input for one Send call.
type Request struct {
	// URLs are the target notification URLs (already validated/allow-listed).
	URLs []string
	// Body is the notification body.
	Body string
	// Title is the notification title.
	Title string
	// NotifyType is one of info|success|warning|failure.
	NotifyType string
	// InputFormat is one of text|markdown|html.
	InputFormat string
	// Attachments are staged local paths or trusted URLs (G3 stages them).
	Attachments []string
	// AttachmentMaxBytes pre-enforces per-attachment size on the API side.
	AttachmentMaxBytes int64
}

// Result reports per-call outcome. Partial failures surface as an error
// from Send (Python maps them to HTTP 424 in G2).
type Result struct {
	// Attempted is the number of target URLs attempted.
	Attempted int
}

// Sender sends Request values via apprise-go with a per-call timeout.
type Sender struct {
	timeout time.Duration
}

// New returns a Sender bounding each call to timeout. Non-positive timeouts
// fall back to 30s.
func New(timeout time.Duration) *Sender {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Sender{timeout: timeout}
}

// Timeout reports the per-call bound.
func (s *Sender) Timeout() time.Duration { return s.timeout }

// Send delivers req via apprise-go AddAll+Send. It fails fast on empty URL
// sets and empty bodies (full body-required-unless-attach rule lands in G2).
func (s *Sender) Send(ctx context.Context, req Request) (Result, error) {
	if len(req.URLs) == 0 {
		return Result{}, fmt.Errorf("notify: no notification URLs configured")
	}
	if strings.TrimSpace(req.Body) == "" && len(req.Attachments) == 0 {
		return Result{}, fmt.Errorf("notify: body is required unless attachments are present")
	}
	client := apprise.New()
	if err := client.AddAll(req.URLs...); err != nil {
		return Result{}, fmt.Errorf("notify: add targets: %w", err)
	}
	opts := []apprise.Option{
		apprise.WithTitle(req.Title),
		apprise.WithNotifyType(apprise.NotifyType(notifyTypeOrDefault(req.NotifyType))),
		apprise.WithInputFormat(inputFormatOrDefault(req.InputFormat)),
	}
	if len(req.Attachments) > 0 {
		opts = append(opts, apprise.WithAttachments(req.Attachments...))
	}
	if req.AttachmentMaxBytes > 0 {
		opts = append(opts, apprise.WithAttachmentMaxBytes(req.AttachmentMaxBytes))
	}
	type sendResult struct {
		err error
	}
	done := make(chan sendResult, 1)
	go func() {
		done <- sendResult{err: client.Send(req.Body, opts...)}
	}()
	timeout := s.timeout
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d < timeout {
			timeout = d
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Result{Attempted: len(req.URLs)}, fmt.Errorf("notify: context done: %w", ctx.Err())
	case r := <-done:
		if r.err != nil {
			return Result{Attempted: len(req.URLs)}, fmt.Errorf("notify: send: %w", r.err)
		}
		return Result{Attempted: len(req.URLs)}, nil
	case <-timer.C:
		return Result{Attempted: len(req.URLs)}, fmt.Errorf("notify: send timed out after %s", timeout)
	}
}

func notifyTypeOrDefault(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "success", "warning", "failure", "info", "":
		if raw == "" {
			return "info"
		}
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "info"
	}
}

func inputFormatOrDefault(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "markdown", "html", "text", "":
		if raw == "" {
			return "text"
		}
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "text"
	}
}
