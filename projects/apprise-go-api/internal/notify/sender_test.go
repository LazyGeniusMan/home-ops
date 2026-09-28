package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newBlackholeServer delays past the per-call timeout before responding,
// so the client gives up on timeout while the handler still finishes.
func newBlackholeServer(d time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(d)
		w.WriteHeader(http.StatusOK)
	}))
}

func TestNewDefaultTimeout(t *testing.T) {
	if got := New(0).Timeout(); got != 30*time.Second {
		t.Errorf("Timeout() = %v, want 30s", got)
	}
	if got := New(5 * time.Second).Timeout(); got != 5*time.Second {
		t.Errorf("Timeout() = %v, want 5s", got)
	}
}

func TestSendNoURLs(t *testing.T) {
	s := New(time.Second)
	if _, err := s.Send(context.Background(), Request{Body: "hi"}); err == nil {
		t.Error("Send(no URLs) = nil, want error")
	}
}

func TestSendEmptyBodyNoAttachments(t *testing.T) {
	s := New(time.Second)
	req := Request{URLs: []string{"json://localhost"}}
	if _, err := s.Send(context.Background(), req); err == nil {
		t.Error("Send(empty body) = nil, want error")
	}
}

func TestSendBadURL(t *testing.T) {
	s := New(5 * time.Second)
	// A lone invalid URL survives filtering (Add's error path) and fails
	// delivery — never silently, and never as ErrNoTargets.
	req := Request{URLs: []string{"://bad-url"}, Body: "hi"}
	if _, err := s.Send(context.Background(), req); err == nil {
		t.Error("Send(bad URL) = nil, want error")
	}
}

func TestSendMixedURLsStillDelivers(t *testing.T) {
	mux := newTestMux()
	srv := newTestServer(mux)
	defer srv.Close()
	s := New(5 * time.Second)
	req := Request{URLs: []string{"://bad-url", testURL(srv)}, Body: "hi"}
	res, err := s.Send(context.Background(), req)
	if err == nil {
		t.Fatal("Send(mixed) = nil, want joined per-target error")
	}
	if res.Attempted != 2 || res.Delivered != 1 {
		t.Errorf("Send(mixed) = %+v, want attempted=2 delivered=1", res)
	}
	if mux.count() != 1 {
		t.Errorf("upstream calls = %d, want 1", mux.count())
	}
}

func TestSendNoTargets(t *testing.T) {
	s := New(time.Second)
	if _, err := s.Send(context.Background(), Request{Body: "hi"}); !isNoTargetsErr(err) {
		t.Errorf("Send(no URLs) = %v, want ErrNoTargets", err)
	}
	req := Request{URLs: []string{"json://localhost"}, Body: "hi", DenyServices: []string{"json"}}
	if _, err := s.Send(context.Background(), req); !isNoTargetsErr(err) {
		t.Errorf("Send(denied) = %v, want ErrNoTargets", err)
	}
}

func isNoTargetsErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrNoTargets)
}

func TestAllowWinsOverDeny(t *testing.T) {
	mux := newTestMux()
	srv := newTestServer(mux)
	defer srv.Close()
	s := New(5 * time.Second)
	// json:// is not the test scheme; use the test server's scheme mapping:
	// allow the json scheme explicitly while denying everything else.
	req := Request{
		URLs:          []string{testURL(srv)},
		Body:          "hi",
		AllowServices: []string{"syslog"},
		DenyServices:  []string{"json"},
	}
	// Allow is exclusive and wins over deny: syslog-only allows nothing.
	if _, err := s.Send(context.Background(), req); !isNoTargetsErr(err) {
		t.Errorf("Send(allow=syslog) = %v, want ErrNoTargets", err)
	}
	// json allowed explicitly while deny lists something else → delivers.
	req.AllowServices = []string{"json"}
	req.DenyServices = []string{"syslog"}
	if _, err := s.Send(context.Background(), req); err != nil {
		t.Errorf("Send(allow=json deny=syslog) = %v, want nil", err)
	}
}

func TestTagFilterAllVsSpecific(t *testing.T) {
	mux := newTestMux()
	srv := newTestServer(mux)
	defer srv.Close()
	s := New(5 * time.Second)
	all, err := ParseTagExpression("all")
	if err != nil {
		t.Fatalf("ParseTagExpression(all) = %v", err)
	}
	req := Request{URLs: []string{testURL(srv)}, Body: "hi", Tag: all}
	if _, err := s.Send(context.Background(), req); err != nil {
		t.Errorf("Send(tag=all) = %v, want nil", err)
	}
	specific, err := ParseTagExpression("mygroup")
	if err != nil {
		t.Fatalf("ParseTagExpression(mygroup) = %v", err)
	}
	req.Tag = specific
	if _, err := s.Send(context.Background(), req); !isNoTargetsErr(err) {
		t.Errorf("Send(tag=mygroup untagged) = %v, want ErrNoTargets", err)
	}
}

func TestServicePolicyHelpers(t *testing.T) {
	if !serviceAllowed("json://localhost", []string{"windows,dbus,gnome,macosx,syslog"}, nil) {
		t.Error("serviceAllowed(json, default deny) = false, want true")
	}
	if serviceAllowed("json://localhost", []string{"json"}, nil) {
		t.Error("serviceAllowed(json, deny=json) = true, want false")
	}
	if !serviceAllowed("json://localhost", []string{"syslog"}, []string{"json"}) {
		t.Error("serviceAllowed(json, allow=json deny=syslog) = false, want true (allow wins)")
	}
	if got := normalizeServiceNames([]string{"JSON, invalid!!, syslog"}); len(got) != 3 {
		t.Errorf("normalizeServiceNames() = %q, want 3 entries", got)
	}
	if !IsSelfRecursionTarget("apprise://localhost/abc123") {
		t.Error("IsSelfRecursionTarget(apprise://) = false, want true")
	}
	if IsSelfRecursionTarget("json://localhost") {
		t.Error("IsSelfRecursionTarget(json://) = true, want false")
	}
}

func TestSendContextCanceled(t *testing.T) {
	s := New(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := Request{URLs: []string{"json://localhost"}, Body: "hi"}
	if _, err := s.Send(ctx, req); err == nil {
		t.Error("Send(canceled ctx) = nil, want error")
	}
}

func TestHelpers(t *testing.T) {
	if got := notifyTypeOrDefault(""); got != "info" {
		t.Errorf("notifyTypeOrDefault() = %q, want info", got)
	}
	if got := inputFormatOrDefault(""); got != "text" {
		t.Errorf("inputFormatOrDefault() = %q, want text", got)
	}
	if got := notifyTypeOrDefault("SUCCESS"); got != "success" {
		t.Errorf("notifyTypeOrDefault(SUCCESS) = %q, want success", got)
	}
}

// pollInFlightZero waits up to timeout for the shared package-global
// inFlight counter to drain to 0, reporting whether it did. A Send that
// times out leaves its worker holding a slot until the backend finishes
// (~2s for the blackhole servers below), so tests must drain before
// resetting the counter: a blind reset while a prior worker still runs
// corrupts that worker's later release and flakes later assertions.
func pollInFlightZero(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for ReportInFlight() != 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

func TestSendTimeoutCountsAndLogs(t *testing.T) {
	before := ReportTimeouts()
	srv := newBlackholeServer(2 * time.Second)
	defer srv.Close()
	s := New(50 * time.Millisecond)
	block := "json://" + hostPort(srv.URL)
	_, err := s.Send(context.Background(), Request{URLs: []string{block}, Body: "hi"})
	if err == nil || !isTimeoutErr(err) {
		t.Fatalf("Send(hung target) = %v, want timeout error", err)
	}
	if got := ReportTimeouts(); got != before+1 {
		t.Errorf("ReportTimeouts() = %d, want %d", got, before+1)
	}
	// The timed-out worker still holds its slot until the blackhole
	// responds (~2s); drain before returning so the next test starts clean.
	if !pollInFlightZero(10 * time.Second) {
		t.Fatal("in-flight never drained after timeout, want 0")
	}
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "timed out")
}

// TestSendTimeoutHoldsSlotUntilWorkerExits is the M-A5 regression test:
// a Send that times out must keep holding its in-flight slot while the
// backend worker is still running (slow backend), releasing only after
// the worker finishes. Otherwise timed-out callers would free slots for
// new sends while their workers still run, over-admitting past maxInFlight.
func TestSendTimeoutHoldsSlotUntilWorkerExits(t *testing.T) {
	if !pollInFlightZero(10 * time.Second) {
		t.Fatal("stale in-flight slots from prior test, want 0")
	}
	inFlightMu.Lock()
	inFlight = 0
	inFlightMu.Unlock()
	defer func() {
		inFlightMu.Lock()
		inFlight = 0
		inFlightMu.Unlock()
	}()
	srv := newBlackholeServer(2 * time.Second)
	defer srv.Close()
	s := New(50 * time.Millisecond)
	block := "json://" + hostPort(srv.URL)
	done := make(chan error, 1)
	go func() {
		_, err := s.Send(context.Background(), Request{URLs: []string{block}, Body: "hi"})
		done <- err
	}()
	// Wait for the timed-out Send to return while its worker still runs.
	var firstErr error
	select {
	case firstErr = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed-out Send did not return")
	}
	if firstErr == nil || !isTimeoutErr(firstErr) {
		t.Fatalf("Send(hung target) = %v, want timeout error", firstErr)
	}
	if got := ReportInFlight(); got != 1 {
		t.Fatalf("in-flight after timeout = %d, want 1 (slot held until worker exits)", got)
	}
	// The worker finishes ~2s later and must release its slot (drains to
	// 0 instead of leaking a held slot forever).
	if !pollInFlightZero(10 * time.Second) {
		t.Fatal("in-flight never drained after worker exit, want 0")
	}
}

func TestSendOverloadFailsFast(t *testing.T) {
	s := New(5 * time.Second)
	if !pollInFlightZero(10 * time.Second) {
		t.Fatal("stale in-flight slots from prior test, want 0")
	}
	inFlightMu.Lock()
	inFlight = maxInFlight
	inFlightMu.Unlock()
	defer func() {
		inFlightMu.Lock()
		inFlight = 0
		inFlightMu.Unlock()
	}()
	mux := newTestMux()
	srv := newTestServer(mux)
	defer srv.Close()
	if _, err := s.Send(context.Background(), Request{URLs: []string{testURL(srv)}, Body: "hi"}); !isOverloadedErr(err) {
		t.Errorf("Send(at cap) = %v, want ErrOverloaded", err)
	}
	if mux.count() != 0 {
		t.Errorf("upstream calls = %d, want 0 (fail fast, no send)", mux.count())
	}
}

func isOverloadedErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrOverloaded)
}
