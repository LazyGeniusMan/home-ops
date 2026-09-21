package attach

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func errUnsupportedFixture() error {
	return fmt.Errorf("json://x: %w", errors.New("attachments unsupported by target"))
}

func TestBodyLimitOverflow(t *testing.T) {
	const capBytes = int64(16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := BodyLimit(w, r, capBytes)
		_, err := io.ReadAll(body)
		if err == nil {
			t.Error("ReadAll(over cap) = nil, want overflow")
			return
		}
		if !IsBodyTooLarge(err) {
			t.Errorf("IsBodyTooLarge() = false for %v", err)
		}
		mapped := MapBodyError(err, capBytes)
		if StatusCodeOf(mapped) != StatusFieldsTooLarge {
			t.Errorf("StatusCodeOf() = %d, want 431", StatusCodeOf(mapped))
		}
	}))
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(strings.Repeat("x", 64)))
	if err != nil {
		t.Fatalf("Post() = %v", err)
	}
	_ = resp.Body.Close()
}

func TestMapBodyErrorPassthrough(t *testing.T) {
	if err := MapBodyError(nil, 10); err != nil {
		t.Errorf("MapBodyError(nil) = %v, want nil", err)
	}
	other := errors.New("boom")
	if err := MapBodyError(other, 10); err != other {
		t.Errorf("MapBodyError(other) = %v, want passthrough", err)
	}
}

func TestParseMultipartAnyField(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, field := range []string{"attach1", "attach2", "file"} {
		fw, err := w.CreateFormFile(field, field+".txt")
		if err != nil {
			t.Fatalf("CreateFormFile() = %v", err)
		}
		_, _ = fw.Write([]byte("content-" + field))
	}
	_ = w.WriteField("body", "hi")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/notify", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	parts, err := ParseMultipart(req, 32<<20)
	if err != nil {
		t.Fatalf("ParseMultipart() = %v", err)
	}
	defer func() { _ = req.MultipartForm.RemoveAll() }()
	if len(parts) != 3 {
		t.Fatalf("len() = %d, want 3", len(parts))
	}
	s := NewStager(testLimits(t.TempDir()))
	staged, err := s.StageRequest(nil, parts)
	if err != nil {
		t.Fatalf("StageRequest(parts) = %v", err)
	}
	t.Cleanup(func() { CleanupAll(staged) })
	if len(staged) != 3 {
		t.Fatalf("staged len = %d, want 3", len(staged))
	}
}

func TestParseMultipartBadMemory(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/notify", nil)
	if _, err := ParseMultipart(req, 0); err == nil {
		t.Error("ParseMultipart(0) = nil, want error")
	}
}

func TestFilePartNil(t *testing.T) {
	inc := FilePart("attachment", nil)
	if inc.Open != nil {
		t.Error("FilePart(nil).Open != nil; staging must 400, not panic")
	}
	s := NewStager(testLimits(t.TempDir()))
	if _, err := s.StageRequest(nil, []Incoming{inc}); err == nil {
		t.Error("StageRequest(nil part) = nil, want error")
	}
}

func TestFormURLs(t *testing.T) {
	got := FormURLs([]string{"  ", "http://a/x.png", "", "http://b/y.png"})
	if len(got) != 2 || got[0] != "http://a/x.png" || got[1] != "http://b/y.png" {
		t.Errorf("FormURLs() = %q, want non-blank only", got)
	}
}
