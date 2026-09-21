// Test helpers for the notify package: an httptest upstream that answers
// every POST with 200, plus the apprise-go URL shape that reaches it.
package notify

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// testMux counts upstream notification deliveries.
type testMux struct {
	mux   *http.ServeMux
	calls atomic.Int64
}

func newTestMux() *testMux {
	m := &testMux{mux: http.NewServeMux()}
	m.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		m.calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	return m
}

func (m *testMux) count() int { return int(m.calls.Load()) }

// newTestServer returns an httptest server wrapped in the json:// URL shape
// apprise-go delivers to (plain http endpoint).
func newTestServer(m *testMux) *httptest.Server {
	return httptest.NewServer(m.mux)
}

// testURL converts an httptest server URL into the json:// target form that
// exercises real network delivery without external services.
func testURL(srv *httptest.Server) string {
	return "json://" + hostPort(srv.URL)
}

func hostPort(raw string) string {
	host := raw
	if i := len("http://"); len(host) > i && host[:i] == "http://" {
		return host[i:]
	}
	return host
}
