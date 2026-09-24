// Package notify wraps the apprise-go notification engine with a
// request-scoped, timeout-bounded sender.
//
// The sender delivers per-target with the allow/deny service policy applied
// up front. Tag routing, attachment staging, and recursion accounting live
// in internal/server.
package notify

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	apprise "github.com/unraid/apprise-go"
)

// Request is the validated stateless notify input for one Send call.
type Request struct {
	// URLs are the target notification URLs. Each URL is added one at a
	// time: invalid entries are skipped so the remaining targets still
	// deliver (Python a_obj.add semantics; zero survivors is ErrNoTargets).
	URLs []string
	// Body is the notification body.
	Body string
	// Title is the notification title.
	Title string
	// NotifyType is one of info|success|warning|failure.
	NotifyType string
	// InputFormat is one of text|markdown|html.
	InputFormat string
	// Tag is the pre-parsed stateless tag filter (OR groups of AND tokens).
	// Nil means no filter: every surviving target is notified.
	Tag []TagGroup
	// Attachments are staged local paths or trusted URLs.
	Attachments []string
	// AttachmentMaxBytes pre-enforces per-attachment size on the API side.
	AttachmentMaxBytes int64
	// DenyServices blocks notification services by name/prefix.
	DenyServices []string
	// AllowServices restricts delivery to these services; non-empty wins
	// over DenyServices (Python apply_global_filters semantics).
	AllowServices []string
	// RecursionCount is forwarded as X-Apprise-Recursion-Count (count+1) on
	// apprise:// self-recursion targets. Zero disables forwarding.
	RecursionCount int
}

// Result reports per-call outcome. Partial failures surface as an error
// from Send (the server maps them to HTTP 424).
type Result struct {
	// Attempted is the number of target URLs attempted.
	Attempted int
	// Delivered counts the targets that accepted the notification.
	Delivered int
}

// ErrNoTargets reports that no valid target URLs survived validation and
// policy filtering (the server maps it to HTTP 204).
var ErrNoTargets = errors.New("notify: no valid URLs provided to notify")

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

// Send delivers req via apprise-go, one target at a time.
//
// Each URL is added individually; entries that fail validation or the
// allow/deny policy are skipped while the survivors still deliver. When no
// target survives, Send returns ErrNoTargets. Per-target Send errors are
// joined: a nil return means every attempted target accepted the
// notification, any error means at least one failed (HTTP 424 upstream).
func (s *Sender) Send(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.Body) == "" && len(req.Attachments) == 0 {
		return Result{}, fmt.Errorf("notify: body is required unless attachments are present")
	}
	base := []apprise.Option{
		apprise.WithTitle(req.Title),
		apprise.WithNotifyType(apprise.NotifyType(notifyTypeOrDefault(req.NotifyType))),
		apprise.WithInputFormat(inputFormatOrDefault(req.InputFormat)),
	}
	if len(req.Attachments) > 0 {
		base = append(base, apprise.WithAttachments(req.Attachments...))
	}
	if req.AttachmentMaxBytes > 0 {
		base = append(base, apprise.WithAttachmentMaxBytes(req.AttachmentMaxBytes))
	}
	targets := filterTargets(req.URLs, req.Tag, req.DenyServices, req.AllowServices)
	if len(targets) == 0 {
		return Result{}, ErrNoTargets
	}
	type sendResult struct {
		delivered int
		err       error
	}
	done := make(chan sendResult, 1)
	go func() {
		var errs []error
		delivered := 0
		for _, target := range targets {
			client := apprise.New()
			if err := client.Add(target); err != nil {
				errs = append(errs, fmt.Errorf("notify: add target: %w", err))
				continue
			}
			opts := append([]apprise.Option(nil), base...)
			if err := client.Send(req.Body, opts...); err != nil {
				errs = append(errs, fmt.Errorf("notify: send: %w", err))
				continue
			}
			delivered++
		}
		done <- sendResult{delivered: delivered, err: errors.Join(errs...)}
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
		return Result{Attempted: len(targets)}, fmt.Errorf("notify: context done: %w", ctx.Err())
	case r := <-done:
		res := Result{Attempted: len(targets), Delivered: r.delivered}
		if r.err != nil {
			return res, r.err
		}
		return res, nil
	case <-timer.C:
		return Result{Attempted: len(targets)}, fmt.Errorf("notify: send timed out after %s", timeout)
	}
}

// filterTargets applies the allow/deny service policy and the stateless tag
// filter to raw URLs, preserving order. Invalid or filtered-out entries are
// dropped without error; callers treat an empty result as ErrNoTargets.
func filterTargets(urls []string, tag []TagGroup, deny, allow []string) []string {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || !serviceAllowed(trimmed, deny, allow) || !tagMatchesURL(trimmed, tag) {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// namePrefixRe mirrors Python apply_global_filters: entries are split on
// [ ,]+ and reduced to a leading [a-z][a-z0-9]+ prefix, lowercased.
var namePrefixRe = regexp.MustCompile(`^[a-z][a-z0-9]+`)

// serviceAllowed reports whether rawURL's scheme survives the allow/deny
// policy. A non-empty allow list is exclusive and wins over deny.
func serviceAllowed(rawURL string, deny, allow []string) bool {
	scheme := urlScheme(rawURL)
	if scheme == "" {
		// Leave malformed URLs to Add's validation error path.
		return true
	}
	if len(normalizeServiceNames(allow)) > 0 {
		return matchServiceName(scheme, normalizeServiceNames(allow))
	}
	if len(normalizeServiceNames(deny)) > 0 {
		return !matchServiceName(scheme, normalizeServiceNames(deny))
	}
	return true
}

// normalizeServiceNames splits entries on commas/whitespace and reduces each
// to its leading lowercase [a-z][a-z0-9]+ prefix; unparseable entries are
// ignored (Python drops non-matching entries the same way).
func normalizeServiceNames(entries []string) []string {
	var out []string
	for _, entry := range entries {
		for _, part := range strings.FieldsFunc(entry, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n'
		}) {
			if name := namePrefixRe.FindString(strings.ToLower(part)); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// matchServiceName reports whether scheme belongs to one of the normalized
// service names. A match covers both directions (json matches jsons and
// vice versa), mirroring Python's per-plugin secure_protocol/protocol set
// intersection.
func matchServiceName(scheme string, names []string) bool {
	scheme = strings.ToLower(scheme)
	for _, name := range names {
		if scheme == name || strings.HasPrefix(scheme, name) || strings.HasPrefix(name, scheme) {
			return true
		}
	}
	return false
}

// urlScheme extracts the lowercase scheme from a raw notification URL.
func urlScheme(raw string) string {
	scheme, _, ok := strings.Cut(strings.TrimSpace(raw), "://")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(scheme))
}

// IsSelfRecursionTarget reports whether rawURL addresses this API itself
// via the apprise:// scheme. Ingress recursion enforcement is authoritative.
func IsSelfRecursionTarget(raw string) bool {
	switch urlScheme(raw) {
	case "apprise", "apprises":
		return true
	default:
		return false
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
