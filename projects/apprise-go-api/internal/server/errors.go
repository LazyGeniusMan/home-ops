// Error contract for the notify API.
//
// Mapping (statusCodeOf mirrors the attach.StatusCodeOf shape: typed match
// first, 500 fallback):
//
//	validation sentinels (bad tag/format/recursion, remap failure) -> 400 Bad Request
//	errRecursionLimit                                  -> 406 Not Acceptable (upstream quirk)
//	errPayloadTooLarge                                 -> 431 (upstream "to large" wording)
//	notify.ErrNoTargets                                -> 204 No Content (zero surviving targets)
//	attach *StatusError / *SendFailure (via errors.As) -> their StatusCode (400/431/424)
//	delivery failures (any other send error)           -> 424 (surfaced by the caller, never statusCodeOf)
//	anything unmapped                                  -> 500 Internal Server Error (statusCodeOf fallback)
//
// There is no 404/422/502 mapping in this service: unknown paths are 404 by
// mux default (stateless-only, no per-key routes), and upstream apprise-api
// surfaces partial delivery failures as 424, never 502.
//
// Single-handling rule: handlers log the error once (s.log with the redacted
// chain for operators) and return only a fixed user-facing string to the
// caller — never the raw error. respondNotify's failure details carry the
// fixed "one or more notifications could not be sent" message with no cause
// suffix; redactCredentials strips URL userinfo (user:pass@) from anything
// that reaches logs or the outbound webhook so user-facing strings carry no
// traces, SQL, PATs, or paths. All Msg/Err constructors use %w (never %v)
// and lowercase messages.
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
