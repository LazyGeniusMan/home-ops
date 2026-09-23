// Error contract for the webhook API.
//
// Mapping (mirrors the apprise attach.StatusError + StatusCodeOf shape):
//
//	invalid key / malformed request body        -> 400 Bad Request
//	secret not found (backend miss, empty value)-> 404 Not Found (lets ESO
//	                                               apply the ExternalSecret
//	                                               deletionPolicy)
//	unprocessable reference (permanent reject)  -> 422 Unprocessable Entity
//	transient backend failure (CLI exec)        -> 502 Bad Gateway
//	push attempt (pull-only provider)           -> 501 Not Implemented
//	anything unmapped                           -> 500 Internal Server Error
//
// Single-handling rule: writeError logs the error once (full chain, for
// operators) and returns only a sanitized {"error"} envelope to the caller.
// publicError strips internals so user-facing strings carry no traces,
// tokens, SQL, PATs, or file paths: upstream failures become a stable
// generic string, sentinel failures keep their short lowercase reason.
//
// Callers must wrap errors with %w (never %v) and keep messages lowercase
// with no trailing punctuation.
package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/provider"
)

// writeError logs err once with its full chain (for operators) and writes
// the sanitized {"error"} envelope with the mapped status. Log OR return,
// never both: the caller-facing envelope carries only the public message.
// Client errors log at warn, server errors at error.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := statusCodeOf(err)
	attrs := []any{slog.Int("status", code), slog.Any("err", err)}
	if code >= 500 {
		s.logger.ErrorContext(r.Context(), "request failed", attrs...)
	} else {
		s.logger.WarnContext(r.Context(), "request failed", attrs...)
	}
	writeJSON(w, code, errorResponse{Error: publicError(err)})
}

// Request-shape sentinels for the statusCodeOf table. Handlers surface
// malformed input directly with these; the envelope reports the public
// message.
var (
	errBadRequest           = errors.New("bad request")
	errMissingKey           = errors.New("missing ?key=pass://{vault}/{item}/{field}")
	errMissingRemoteRefKey  = errors.New("missing remoteref.key")
	errMethodNotAllowed     = errors.New("method not allowed: use get or post")
	errMethodNotAllowedPush = errors.New("method not allowed: use post")
)

// statusCodeOf maps webhook errors to HTTP statuses. It mirrors the apprise
// attach.StatusCodeOf shape: typed match first (errors.Is/As over the %w
// chain), 500 fallback.
func statusCodeOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, errBadRequest) ||
		errors.Is(err, errMissingKey) ||
		errors.Is(err, errMissingRemoteRefKey) ||
		errors.Is(err, provider.ErrInvalidKey) {
		return http.StatusBadRequest
	}
	if errors.Is(err, provider.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, provider.ErrUnprocessable) {
		return http.StatusUnprocessableEntity
	}
	if errors.Is(err, provider.ErrPushUnimplemented) {
		return http.StatusNotImplemented
	}
	if errors.Is(err, errMethodNotAllowed) || errors.Is(err, errMethodNotAllowedPush) {
		return http.StatusMethodNotAllowed
	}
	var statusCoder interface{ StatusCode() int }
	if errors.As(err, &statusCoder) {
		if code := statusCoder.StatusCode(); code >= 400 && code < 600 {
			return code
		}
	}
	if errors.Is(err, provider.ErrUpstream) {
		return http.StatusBadGateway
	}
	// Genuinely unknown failures (no sentinel, no StatusCode carrier, no
	// backend origin) are internal bugs → 500. Transient backend failures
	// default to 502 (ESO retries).
	if unmapped(err) {
		return http.StatusInternalServerError
	}
	if isBackend(err) {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// errUnmapped is the test-only marker exercising the 500 fallback path.
// Production code never constructs it; it lives here (not in _test.go) so
// both unmapped() and the table test reference the same type.
type errUnmapped struct{}

// Error implements error.
func (errUnmapped) Error() string { return "test-only unmapped" }

// errUnmappedTestOnly builds the 500-fallback probe error.
func errUnmappedTestOnly() error { return errUnmapped{} }

// backendSentinel reports whether err carries a permanent-failure sentinel
// (input shape, miss, permanent reject, push misuse): such errors are
// classified by the table above, never as transient backend failures.
func backendSentinel(err error) bool {
	return err == nil ||
		errors.Is(err, errBadRequest) ||
		errors.Is(err, errMissingKey) ||
		errors.Is(err, errMissingRemoteRefKey) ||
		errors.Is(err, errMethodNotAllowed) ||
		errors.Is(err, errMethodNotAllowedPush) ||
		errors.Is(err, provider.ErrInvalidKey) ||
		errors.Is(err, provider.ErrNotFound) ||
		errors.Is(err, provider.ErrUnprocessable) ||
		errors.Is(err, provider.ErrPushUnimplemented)
}

// isBackend reports whether err originates below the provider (exec or
// resolver failure without a permanent-failure sentinel or StatusCode
// carrier).
func isBackend(err error) bool {
	if err == nil || backendSentinel(err) {
		return false
	}
	var statusCoder interface{ StatusCode() int }
	return !errors.As(err, &statusCoder)
}

// unmapped reports the test-only 500-fallback probe: the errUnmapped marker
// with no wrapping. Everything else falls through to the backend-502 or
// final-500 branches of statusCodeOf.
func unmapped(err error) bool {
	if _, ok := err.(errUnmapped); ok {
		return true
	}
	return false
}

// publicError sanitizes err for the {"error"} envelope: sentinel failures
// keep their short lowercase reason; transient backend failures map to a
// stable generic string so envelopes carry no traces, tokens, or paths.
func publicError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, errMissingKey):
		return "missing ?key=pass://{vault}/{item}/{field}"
	case errors.Is(err, errMissingRemoteRefKey):
		return "missing remoteref.key"
	case errors.Is(err, errBadRequest):
		return "bad request"
	case errors.Is(err, errMethodNotAllowed):
		return "method not allowed: use get or post"
	case errors.Is(err, errMethodNotAllowedPush):
		return "method not allowed: use post"
	case errors.Is(err, provider.ErrInvalidKey):
		return "invalid key: want pass://{vault}/{item}/{field}"
	case errors.Is(err, provider.ErrNotFound):
		// 404 lets ESO apply the ExternalSecret deletionPolicy.
		return "secret not found"
	case errors.Is(err, provider.ErrUnprocessable):
		return "unprocessable reference"
	case errors.Is(err, provider.ErrPushUnimplemented):
		return "push is not implemented (pull-only provider)"
	case errors.Is(err, provider.ErrUpstream) || isBackend(err):
		return "failed to resolve secret"
	}
	msg := strings.TrimSpace(firstLine(err.Error()))
	msg = strings.ToLower(msg)
	if msg == "" {
		return "request failed"
	}
	return msg
}

// firstLine drops wrapped-cause detail past the first newline so envelopes
// stay single-line and free of traces/paths.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
