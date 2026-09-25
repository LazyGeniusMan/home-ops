// Error contract: soft (transient) → 502 (ExternalDNS retries), hard
// (permanent) → 422, decode failures → 400, else 500. Single-handling rule:
// log once, return a sanitized envelope; %w, lowercase.
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
