// Error contract for the webhook API.
//
// Interop: provider.NewSoftError (sigs.k8s.io/external-dns/provider) marks
// transient NetBird failures. Soft errors keep their %w chain so
// errors.Is(err, provider.SoftError) holds through any wrapping; callers
// must wrap with %w (never %v) and keep messages lowercase.
//
// Mapping (mirrors the apprise StatusError + StatusCodeOf shape):
//
//	soft (transient NetBird/API failure) -> 502 Bad Gateway, ExternalDNS retries
//	hard (permanent: no matching zone, bad input) -> 422 Unprocessable Entity
//	decode failures in handlers -> 400 Bad Request
//	anything unmapped -> 500 Internal Server Error (statusCodeOf fallback)
//
// Single-handling rule: handlers log the error once (s.log with the full
// chain for operators) and return only a sanitized {"error"} envelope to
// the caller. publicError strips provider internals so user-facing strings
// carry no traces, tokens, or file paths: soft errors become
// "netbird api temporarily unavailable", hard errors keep their short
// lowercase reason (zone-miss messages only name the DNS name, never paths
// or secrets).
package server

import (
	"errors"
	"net/http"
	"strings"

	"sigs.k8s.io/external-dns/provider"

	nbprovider "github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
)

// statusCodeOf maps webhook errors to HTTP statuses. It mirrors the apprise
// attach.StatusCodeOf shape: typed match first, 500 fallback.
func statusCodeOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if isSoft(err) {
		return http.StatusBadGateway
	}
	var apiErr interface{ StatusCode() int }
	if errors.As(err, &apiErr) {
		if code := apiErr.StatusCode(); code >= 400 && code < 600 {
			return code
		}
	}
	if errors.Is(err, errBadRequest) {
		return http.StatusBadRequest
	}
	if errors.Is(err, errNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, nbprovider.ErrNoMatchingZone) {
		return http.StatusUnprocessableEntity
	}
	// Permanent provider errors (e.g. validation failures) are hard
	// failures: ExternalDNS must not retry them.
	return http.StatusUnprocessableEntity
}

// errBadRequest and errNotFound are sentinel matches for the statusCodeOf
// table; the webhook handlers surface decode failures directly as 400.
var (
	errBadRequest = errors.New("bad request")
	errNotFound   = errors.New("not found")
)

// publicError sanitizes err for the {"error"} envelope: soft failures map
// to a stable generic string, hard failures keep their short lowercase
// reason with any provider-internal detail trimmed.
func publicError(err error) string {
	if err == nil {
		return ""
	}
	if isSoft(err) {
		return "netbird api temporarily unavailable"
	}
	msg := strings.TrimSpace(firstLine(err.Error()))
	msg = strings.ToLower(msg)
	msg = strings.TrimPrefix(msg, "netbird: ")
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

// isSoft reports whether err carries the external-dns soft-error marker
// (transient: NetBird 429/5xx, transport failures). Soft errors surface as
// 502 so ExternalDNS retries; everything else is permanent (422).
func isSoft(err error) bool {
	return errors.Is(err, provider.SoftError)
}
