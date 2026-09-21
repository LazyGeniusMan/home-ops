package notify

import (
	"context"
	"testing"
	"time"
)

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
	return err == ErrNoTargets
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
