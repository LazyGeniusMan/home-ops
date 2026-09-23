package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigs.k8s.io/external-dns/provider"

	nbprovider "github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
)

// statusErr is a StatusCode()-carrying error mirroring the apprise
// StatusError shape (typed status, %w-unwrappable cause).
type statusErr struct {
	code int
	msg  string
	err  error
}

func (e *statusErr) Error() string   { return e.msg + ": " + e.err.Error() }
func (e *statusErr) Unwrap() error   { return e.err }
func (e *statusErr) StatusCode() int { return e.code }

func TestStatusCodeOf(t *testing.T) {
	soft := provider.NewSoftError(errors.New("netbird: connection reset"))
	wrappedSoft := fmt.Errorf("records: %w", soft)
	hard := fmt.Errorf("%w for %q", nbprovider.ErrNoMatchingZone, "host.other.net")
	notFound := fmt.Errorf("lookup: %w", errNotFound)
	badReq := fmt.Errorf("decode: %w", errBadRequest)
	coded := &statusErr{code: http.StatusNotFound, msg: "attach", err: errors.New("missing")}
	plain := errors.New("boom")

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"soft", soft, http.StatusBadGateway},
		{"wrapped soft", wrappedSoft, http.StatusBadGateway},
		{"hard zone miss", hard, http.StatusUnprocessableEntity},
		{"plain hard", plain, http.StatusUnprocessableEntity},
		{"sentinel 400", badReq, http.StatusBadRequest},
		{"sentinel 404", notFound, http.StatusNotFound},
		{"status coder 404", coded, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusCodeOf(tc.err); got != tc.want {
				t.Errorf("statusCodeOf = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWriteErrorSanitizes(t *testing.T) {
	s := testServer()
	soft := provider.NewSoftError(fmt.Errorf("netbird: get https://api.netbird.io/api/dns/zones: token abc123 at /etc/secrets/pat"))

	rec := httptest.NewRecorder()
	s.writeError(rec, soft)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("soft status = %d, want 502", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, leak := range []string{"abc123", "/etc/secrets", "https://"} {
		if strings.Contains(body["error"], leak) {
			t.Errorf("soft envelope leaks %q: %q", leak, body["error"])
		}
	}
	if body["error"] == "" {
		t.Error("soft envelope must carry a generic message")
	}

	hard := fmt.Errorf("%w for %q", nbprovider.ErrNoMatchingZone, "host.other.net")
	rec = httptest.NewRecorder()
	s.writeError(rec, hard)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("hard status = %d, want 422", rec.Code)
	}
	body = nil
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body["error"], "no matching zone") {
		t.Errorf("hard envelope = %q, want zone-miss reason", body["error"])
	}
	for _, leak := range []string{"Token", "trace", "/etc/"} {
		if strings.Contains(body["error"], leak) {
			t.Errorf("hard envelope leaks %q: %q", leak, body["error"])
		}
	}
}

func TestSoftChainSurvivesWrap(t *testing.T) {
	err := fmt.Errorf("apply: %w", provider.NewSoftError(errors.New("netbird: 503")))
	if !errors.Is(err, provider.SoftError) {
		t.Fatal("errors.Is must find provider.SoftError through %w wrap")
	}
	if got := statusCodeOf(err); got != http.StatusBadGateway {
		t.Errorf("wrapped soft maps to %d, want 502", got)
	}
}
