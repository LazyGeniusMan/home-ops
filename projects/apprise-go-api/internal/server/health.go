// Health and readiness probes plus the attach-dir writability cache.
//
// GET /healthz is the liveness probe: it returns {"status":"ok"} with zero
// downstream calls (no sender, no filesystem, no attach probe) in <50ms.
//
// GET /readyz is the readiness probe: it checks the attach staging dir via
// the TTL cache and returns {"status":"ok"} when writable, or 503
// {"status":"not_ready","failing":"attach-dir"} when not. Probe failures are
// logged server-side and never expose traces or paths.
//
// /details stays a domain endpoint (service catalog), never a probe.
//
// The attach writability probe (MkdirAll + CreateTemp) has a side effect per
// call, so it must never run per scrape: cachedAttachProbe serves the
// result from a TTL cache (attachProbeTTL), and /metrics reads the same
// cached gauge.
package server

import (
	"net/http"
	"sync"
	"time"
)

// attachProbeTTL bounds writability-probe side effects: at most one
// MkdirAll+CreateTemp per TTL window per process.
const attachProbeTTL = 30 * time.Second

var attachCache struct {
	sync.Mutex
	dir      string
	resolved string
	writable bool
	issue    string
	at       time.Time
}

// cachedAttachProbe serves the writability result from the TTL cache,
// refreshing the probe at most every attachProbeTTL. It also refreshes the
// attachWritable gauge so /metrics never probes per scrape.
func cachedAttachProbe(dir string) (resolved string, canWrite bool, issue string) {
	attachCache.Lock()
	defer attachCache.Unlock()
	if attachCache.dir == dir && time.Since(attachCache.at) < attachProbeTTL {
		return attachCache.resolved, attachCache.writable, attachCache.issue
	}
	resolved, canWrite, issue = attachProbe(dir)
	attachCache.dir = dir
	attachCache.resolved = resolved
	attachCache.writable = canWrite
	attachCache.issue = issue
	attachCache.at = time.Now()
	if canWrite {
		attachWritable.Set(1)
	} else {
		attachWritable.Set(0)
	}
	return resolved, canWrite, issue
}

// resetAttachCache clears the TTL cache (tests only).
func resetAttachCache() {
	attachCache.Lock()
	attachCache.dir = ""
	attachCache.resolved = ""
	attachCache.writable = false
	attachCache.issue = ""
	attachCache.at = time.Time{}
	attachCache.Unlock()
}

// handleHealthz is the liveness probe: static JSON, zero downstream calls.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleReadyz probes readiness via the attach-dir TTL cache. A failing
// dependency returns 503 {"status":"not_ready","failing":"<dep>"}; the
// underlying cause is logged server-side, never exposed.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if failing, ready := s.readyCheck(); !ready {
		s.log.Warn("readyz: dependency not ready", "failing", failing)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "not_ready",
			"failing": failing,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// readyCheck runs the readiness dependency probe (attach-dir writability via
// the TTL cache). It returns ("", true) when ready.
func (s *Server) readyCheck() (string, bool) {
	_, canWrite, _ := cachedAttachProbe(s.cfg.AttachDir)
	if !canWrite {
		return "attach-dir", false
	}
	return "", true
}
