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
	req := Request{URLs: []string{"://bad-url"}, Body: "hi"}
	if _, err := s.Send(context.Background(), req); err == nil {
		t.Error("Send(bad URL) = nil, want error")
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
