// Error contract: sentinels → 400, recursion → 406 (quirk), too-large →
// 431, no-targets → 204, attach → its code, delivery → 424, else 500.
// Log once redacted, return fixed string; %w, lowercase.
package server

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/attach"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

// Validation sentinels. Messages stay lowercase (internal only); the
// user-facing HTTP bodies are the fixed Python-parity literals at the fail()
// call sites, never these strings.
var (
	// errInvalidTag marks a tag/tags value outside the tag grammar.
	errInvalidTag = errors.New("notify: unsupported characters in tag definition")
	// errInvalidFormat marks a body format outside text|markdown|html.
	errInvalidFormat = errors.New("notify: invalid body input format")
	// errInvalidRecursion marks a negative or unparseable
	// X-Apprise-Recursion-Count value.
	errInvalidRecursion = errors.New("notify: invalid recursion value")
	// errRecursionLimit marks a recursion count over APPRISE_RECURSION_MAX.
	errRecursionLimit = errors.New("notify: recursion limit reached")
	// errRemapFailed wraps ':' remap rule/apply failures.
	errRemapFailed = errors.New("notify: payload field mapping failed")
)

// statusCodeOf maps notify-path errors to HTTP statuses. Delivery failures
// (non-nil, non-NoTargets send errors) are NOT mapped here: the caller
// surfaces them as 424 directly, mirroring Python's partial-failure shape.
func statusCodeOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, errPayloadTooLarge) {
		return http.StatusRequestHeaderFieldsTooLarge
	}
	if errors.Is(err, errInvalidTag) ||
		errors.Is(err, errInvalidFormat) ||
		errors.Is(err, errInvalidRecursion) ||
		errors.Is(err, errRemapFailed) {
		return http.StatusBadRequest
	}
	if errors.Is(err, errRecursionLimit) {
		return http.StatusNotAcceptable
	}
	if errors.Is(err, notify.ErrNoTargets) {
		return http.StatusNoContent
	}
	var se *attach.StatusError
	if errors.As(err, &se) {
		return se.StatusCode()
	}
	var sf *attach.SendFailure
	if errors.As(err, &sf) {
		return sf.StatusCode()
	}
	return http.StatusInternalServerError
}

// userinfoRe matches URL userinfo (user[:pass]@) so credentials embedded in
// notification or attachment URLs never reach logs, responses, or webhooks.
var userinfoRe = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s?#]+@`)

// redactCredentials replaces URL userinfo with ***. Non-URL text passes
// through unchanged.
func redactCredentials(s string) string {
	return userinfoRe.ReplaceAllString(s, `${1}***@`)
}
