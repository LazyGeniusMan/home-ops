// Table + golden tests for POST /notify, mirroring Python
// test_stateless_notify.py. The sender is a fake: upstream delivery is never
// attempted in these tests — the fake records the validated request and
// returns scripted outcomes so status codes and response shapes are asserted
// exactly.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

// fakeSender scripts Send outcomes and records the last request.
type fakeSender struct {
	mu       sync.Mutex
	last     notify.Request
	calls    int
	err      error
	noTarget bool
}

func (f *fakeSender) Send(_ context.Context, req notify.Request) (notify.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = req
	f.calls++
	if f.noTarget {
		return notify.Result{}, notify.ErrNoTargets
	}
	if f.err != nil {
		return notify.Result{Attempted: len(req.URLs)}, f.err
	}
	return notify.Result{Attempted: len(req.URLs), Delivered: len(req.URLs)}, nil
}

func (f *fakeSender) Timeout() time.Duration { return time.Second }

func (f *fakeSender) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = 0
	f.err = nil
	f.noTarget = false
	f.last = notify.Request{}
}

func (f *fakeSender) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

type notifyHarness struct {
	srv  *Server
	fake *fakeSender
}

func newNotifyHarness(cfg config.Config) *notifyHarness {
	if cfg.CallTimeoutSecs == 0 {
		cfg.CallTimeoutSecs = 30
	}
	// Default the recursion ceiling to 1 (Python APPRISE_RECURSION_MAX).
	// Tests needing another ceiling set RecursionMax explicitly.
	if cfg.RecursionMax == 0 {
		cfg.RecursionMax = 1
	}
	// cfg zero values are valid choices (RecursionMax 0 = no recursion);
	// only fill functional defaults the handler divides by or caps with.
	if cfg.WebhookMappingMaxDepth == 0 {
		cfg.WebhookMappingMaxDepth = 5
	}
	if cfg.AttachSizeMB == 0 {
		cfg.AttachSizeMB = 200
	}
	if cfg.UploadMaxMemorySizeMB == 0 {
		cfg.UploadMaxMemorySizeMB = 3
	}
	fake := &fakeSender{}
	s := New(cfg, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	s.sender = fake
	return &notifyHarness{srv: s, fake: fake}
}

func doPost(h *notifyHarness, target, contentType, body string, headers map[string]string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, target, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestNotifyValidationTable(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		headers     map[string]string
		target      string
		wantStatus  int
		wantCalls   int
		wantBody    string
	}{
		{
			name:        "form urls+body 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "form markdown 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}, "format": {"markdown"}}.Encode(),
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "form advanced tag 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}, "tag": {"family:2, 3:friends:4"}}.Encode(),
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "form tag from query 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			target:      "/notify?tag=family:2",
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "form tags from query 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			target:      "/notify?tags=3:friends:4",
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "form invalid tag query 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			target:      "/notify?tag=family:",
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json tag list passthrough 200",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi","tag":["family","3:friends:4"]}`,
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "json non-string tag 400",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi","tag":{"name":"family"}}`,
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json garbage 400",
			contentType: "application/json",
			body:        `{`,
			target:      "/notify/",
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "xml content type 400",
			contentType: "application/xml",
			body:        `{}`,
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json empty object 400",
			contentType: "application/json",
			body:        `{}`,
			target:      "/notify/",
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json type only 400",
			contentType: "application/json",
			body:        `{"type":"warning"}`,
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json invalid format 400",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi","format":"invalid"}`,
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
			// Python rejects a bad format after the minimum-requirements
			// gate (api/views.py:2040) with its own message.
			wantBody: "An invalid body input format was specified",
		},
		{
			name:        "json empty format 200",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi","format":""}`,
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "form invalid format choice 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}, "format": {"invalid_format_xyz"}}.Encode(),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json get params title format type 200",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi"}`,
			target:      "/notify/?title=my%20title&format=text&type=info",
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			name:        "json invalid type 400",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi","type":"bogus"}`,
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
			// Python StatelessNotifyView bundles the type check into the
			// minimum-requirements gate (api/views.py:2016), so a bad type
			// reports "Payload lacks minimum requirements".
			wantBody: "Payload lacks minimum requirements",
		},
		{
			name:        "form attach alias bad attachment 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}, "attach": {"https://localhost/invalid.png"}}.Encode(),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json attachments alias bad attachment 400",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","body":"hi","attachments":"https://localhost/invalid.png"}`,
			target:      "/notify/",
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "json attach-only aliases 400 bad attachment",
			contentType: "application/json",
			body:        `{"urls":"json://localhost","attach":"https://localhost/invalid.png"}`,
			target:      "/notify/",
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "form missing body+attach 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}}.Encode(),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "recursion over limit 406",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			headers:     map[string]string{"X-Apprise-Recursion-Count": "2", "X-Apprise-ID": "abc123"},
			wantStatus:  http.StatusNotAcceptable,
			wantCalls:   0,
		},
		{
			name:        "recursion negative 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			headers:     map[string]string{"X-Apprise-Recursion-Count": "-1"},
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "recursion unparseable 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			headers:     map[string]string{"X-Apprise-Recursion-Count": "invalid"},
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			name:        "recursion within limit 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			headers:     map[string]string{"X-Apprise-Recursion-Count": "1", "X-Apprise-ID": "abc123"},
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
		{
			// Remap failure: nested source with a missing leaf
			// (event.missing is not in the payload) → 400, no send.
			name:        "remap failure 400",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"event": {"x"}, "urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			target:      "/notify/?:event.missing=body",
			wantStatus:  http.StatusBadRequest,
			wantCalls:   0,
		},
		{
			// Mappable stub rule passes through untouched (full engine G4).
			name:        "remap stub passthrough 200",
			contentType: "application/x-www-form-urlencoded",
			body:        url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
			target:      "/notify/?:payload=body",
			wantStatus:  http.StatusOK,
			wantCalls:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newNotifyHarness(config.Config{})
			h.fake.reset()
			target := tc.target
			if target == "" {
				target = "/notify"
			}
			rec := doPost(h, target, tc.contentType, tc.body, tc.headers)
			if rec.Code != tc.wantStatus {
				t.Errorf("POST %s = %d, want %d (body %q)", target, rec.Code, tc.wantStatus, rec.Body.String())
			}
			h.fake.mu.Lock()
			calls := h.fake.calls
			h.fake.mu.Unlock()
			if calls != tc.wantCalls {
				t.Errorf("sender calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want substring %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestNotifyMethodsAndPaths(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	for _, path := range []string{"/notify", "/notify/"} {
		rec := doPost(h, path, "application/json", `{"urls":"json://localhost","body":"hi"}`, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s = %d, want 200", path, rec.Code)
		}
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			req := httptest.NewRequest(method, path, nil)
			rec := httptest.NewRecorder()
			h.srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405", method, path, rec.Code)
			}
		}
	}
}

func TestNotifyStatelessURLsFallback(t *testing.T) {
	h := newNotifyHarness(config.Config{StatelessURLs: "json://localhost"})
	rec := doPost(h, "/notify", "application/x-www-form-urlencoded",
		url.Values{"body": {"hi"}}.Encode(), nil)
	if rec.Code != http.StatusOK {
		t.Errorf("POST fallback = %d, want 200", rec.Code)
	}
	h.fake.mu.Lock()
	urls := h.fake.last.URLs
	h.fake.mu.Unlock()
	if len(urls) != 1 || urls[0] != "json://localhost" {
		t.Errorf("fallback urls = %v, want [json://localhost]", urls)
	}
}

func TestNotifyNoValidURLs204(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	h.fake.mu.Lock()
	h.fake.noTarget = true
	h.fake.mu.Unlock()
	// Empty urls string → no targets → 204, sender attempted zero times.
	rec := doPost(h, "/notify", "application/json", `{"urls":"","body":"hi"}`, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("POST empty urls = %d, want 204", rec.Code)
	}
}

func TestNotifyRemapFlowThrough(t *testing.T) {
	// Flat form remap: ?:subject=title&:payload=body maps webhook field
	// names onto the notify request (body wins over query fallbacks).
	h := newNotifyHarness(config.Config{})
	rec := doPost(h, "/notify/?:subject=title&:payload=body",
		"application/x-www-form-urlencoded",
		url.Values{"urls": {"json://localhost"}, "subject": {"Subj"}, "payload": {"Content"}}.Encode(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("form remap = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	last := h.fake.last
	h.fake.mu.Unlock()
	if last.Title != "Subj" || last.Body != "Content" {
		t.Errorf("form remap sent title=%q body=%q, want Subj/Content", last.Title, last.Body)
	}

	// JSON variant with &:href=urls: subject/href/payload remap into
	// title/urls/body.
	h.fake.reset()
	rec = doPost(h, "/notify/?:subject=title&:payload=body&:href=urls",
		"application/json",
		`{"subject":"JSubj","payload":"JContent","href":"json://localhost"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("JSON remap = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	last = h.fake.last
	h.fake.mu.Unlock()
	if last.Title != "JSubj" || last.Body != "JContent" {
		t.Errorf("JSON remap sent title=%q body=%q, want JSubj/JContent", last.Title, last.Body)
	}
	if len(last.URLs) != 1 || last.URLs[0] != "json://localhost" {
		t.Errorf("JSON remap sent urls=%v, want [json://localhost]", last.URLs)
	}

	// Nested source: ?:event.title=title resolves into the payload.
	h.fake.reset()
	rec = doPost(h, "/notify/?:event.title=title",
		"application/json",
		`{"urls":"json://localhost","body":"hi","event":{"title":"Nested"}}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("nested remap = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	last = h.fake.last
	h.fake.mu.Unlock()
	if last.Title != "Nested" {
		t.Errorf("nested remap sent title=%q, want Nested", last.Title)
	}
}

func TestNotifyPartialFailure424Vs204(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	// Golden: upstream failure → 424 with error+details JSON (Accept: json).
	h.fake.fail(errors.New("boom"))
	rec := doPost(h, "/notify", "application/json",
		`{"urls":"json://a,json://b","body":"hi"}`,
		map[string]string{"Accept": "application/json"})
	if rec.Code != http.StatusFailedDependency {
		t.Fatalf("POST failing = %d, want 424", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("424 body is not JSON: %v", err)
	}
	if payload["error"] != "One or more notifications could not be sent" {
		t.Errorf("424 error = %v, want upstream-failure message", payload["error"])
	}
	if _, ok := payload["details"]; !ok {
		t.Error("424 JSON missing details")
	}

	// Golden: no valid URLs → 204 (distinct from 424 above).
	h.fake.reset()
	h.fake.mu.Lock()
	h.fake.noTarget = true
	h.fake.mu.Unlock()
	rec = doPost(h, "/notify", "application/json", `{"urls":"","body":"hi"}`, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("POST empty urls = %d, want 204 (not 424)", rec.Code)
	}
}

func TestNotifyJSONSuccessShape(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	rec := doPost(h, "/notify", "application/json",
		`{"urls":"json://localhost","body":"hi"}`,
		map[string]string{"Accept": "application/json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d, want 200", rec.Code)
	}
	var payload struct {
		Error   *string    `json:"error"`
		Details [][]string `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("200 body is not JSON: %v", err)
	}
	if payload.Error != nil {
		t.Errorf("error = %q, want null", *payload.Error)
	}
	if len(payload.Details) != 1 || len(payload.Details[0]) != 3 {
		t.Fatalf("details = %v, want one [level,date,message] triple", payload.Details)
	}
}

func TestNotifyHTMLResponseBlock(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	rec := doPost(h, "/notify", "application/x-www-form-urlencoded",
		url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(),
		map[string]string{"Accept": "text/html"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<ul class="logs">`) {
		t.Errorf("body = %q, want <ul class=\"logs\">", body)
	}
}

func TestNotifyTextResponse(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	rec := doPost(h, "/notify", "application/x-www-form-urlencoded",
		url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Delivered Stateless Notification(s)") {
		t.Errorf("body = %q, want delivery line", body)
	}
}

func TestNotifyJSONTooLarge431(t *testing.T) {
	h := newNotifyHarness(config.Config{UploadMaxMemorySizeMB: 1})
	big := strings.Repeat("x", (1<<20)+10)
	rec := doPost(h, "/notify", "application/json",
		fmt.Sprintf(`{"urls":"json://localhost","body":%q}`, big), nil)
	if rec.Code != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("POST oversize = %d, want 431", rec.Code)
	}
	h.fake.mu.Lock()
	calls := h.fake.calls
	h.fake.mu.Unlock()
	if calls != 0 {
		t.Errorf("sender calls = %d, want 0", calls)
	}
}

func TestNotifyAllowDenyPolicy(t *testing.T) {
	h := newNotifyHarness(config.Config{DenyServices: []string{"json"}})
	h.fake.mu.Lock()
	h.fake.noTarget = false
	h.fake.mu.Unlock()
	// With json denied at the policy layer the fake still gets called (it
	// does not enforce policy); assert the request carries the policy so
	// the real sender filters. Use noTarget to simulate filtered outcome.
	rec := doPost(h, "/notify", "application/json", `{"urls":"json://localhost","body":"hi"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d, want 200", rec.Code)
	}
	h.fake.mu.Lock()
	deny := h.fake.last.DenyServices
	h.fake.mu.Unlock()
	if len(deny) != 1 || deny[0] != "json" {
		t.Errorf("deny = %v, want [json]", deny)
	}
}

func TestNotifyWebhookFires(t *testing.T) {
	var got struct {
		Source string `json:"source"`
		Status int    `json:"status"`
		Output string `json:"output"`
	}
	var calls int
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()
	h := newNotifyHarness(config.Config{WebhookURL: hook.URL})
	rec := doPost(h, "/notify", "application/json", `{"urls":"json://localhost","body":"hi"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d, want 200", rec.Code)
	}
	if calls != 1 {
		t.Fatalf("webhook calls = %d, want 1", calls)
	}
	if got.Status != 0 {
		t.Errorf("webhook status = %d, want 0", got.Status)
	}
	if got.Source == "" {
		t.Error("webhook source empty, want remote addr")
	}
}

func TestNotifyLogLevelHeaders(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	for _, level := range []string{"CRITICAL", "ERROR", "WARNING", "INFO", "DEBUG", "TRACE", "INVALID"} {
		h.fake.reset()
		rec := doPost(h, "/notify", "application/json",
			`{"urls":"json://localhost","body":"hi"}`,
			map[string]string{"X-Apprise-Log-Level": level})
		if rec.Code != http.StatusOK {
			t.Errorf("log-level %s = %d, want 200", level, rec.Code)
		}
	}
}

// TestNotifyMultipartAttachmentStaged posts a multipart file part and
// asserts it is staged under the attach dir, handed to the sender as a
// local path, and cleaned up before the response returns.
func TestNotifyMultipartAttachmentStaged(t *testing.T) {
	dir := t.TempDir()
	h := newNotifyHarness(config.Config{AttachDir: dir})
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, "hello attach"); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.WriteField("urls", "json://localhost"); err != nil {
		t.Fatalf("write urls field: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/notify", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST multipart = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	got := h.fake.last.Attachments
	h.fake.mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("sender attachments = %v, want one staged path", got)
	}
	if _, err := os.Stat(got[0]); !os.IsNotExist(err) {
		t.Errorf("staged path %q still exists, want cleaned up", got[0])
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read attach dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("attach dir has %d entries, want empty", len(entries))
	}
}

// TestNotifyRemoteAttachmentDenied asserts SSRF deny-first: a remote URL on
// the attach denylist is a 400 and never reaches the sender.
func TestNotifyRemoteAttachmentDenied(t *testing.T) {
	h := newNotifyHarness(config.Config{AttachRejectURL: "example.*"})
	rec := doPost(h, "/notify", "application/json",
		`{"urls":"json://localhost","body":"hi","attach":"https://example.com/file.png"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST denied attach = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Bad Attachment") {
		t.Errorf("body = %q, want Bad Attachment", rec.Body.String())
	}
	h.fake.mu.Lock()
	calls := h.fake.calls
	h.fake.mu.Unlock()
	if calls != 0 {
		t.Errorf("sender calls = %d, want 0", calls)
	}
}

// TestNotifyJSONBase64Attachment asserts a JSON {base64,filename} dict is
// staged and sent, and that bad base64 is a 400.
func TestNotifyJSONBase64Attachment(t *testing.T) {
	dir := t.TempDir()
	h := newNotifyHarness(config.Config{AttachDir: dir})
	rec := doPost(h, "/notify", "application/json",
		`{"urls":"json://localhost","body":"hi","attach":{"base64":"aGVsbG8=","filename":"hi.txt"}}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST base64 attach = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	got := h.fake.last.Attachments
	maxBytes := h.fake.last.AttachmentMaxBytes
	h.fake.mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("sender attachments = %v, want one staged path", got)
	}
	if maxBytes <= 0 {
		t.Errorf("sender AttachmentMaxBytes = %d, want positive per-file cap", maxBytes)
	}
	if _, err := os.Stat(got[0]); !os.IsNotExist(err) {
		t.Errorf("staged path %q still exists, want cleaned up", got[0])
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read attach dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("attach dir has %d entries, want empty", len(entries))
	}

	h.fake.reset()
	rec = doPost(h, "/notify", "application/json",
		`{"urls":"json://localhost","body":"hi","attach":{"base64":"!!!not-base64!!!","filename":"hi.txt"}}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST bad base64 = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	calls := h.fake.calls
	h.fake.mu.Unlock()
	if calls != 0 {
		t.Errorf("sender calls = %d, want 0", calls)
	}
}

// TestNotifyBodyOmittedWithAttachment asserts an attach-only request (no
// body) passes the minimum-requirements rule via staged attachments.
func TestNotifyBodyOmittedWithAttachment(t *testing.T) {
	dir := t.TempDir()
	h := newNotifyHarness(config.Config{AttachDir: dir})
	rec := doPost(h, "/notify", "application/json",
		`{"urls":"json://localhost","attach":{"base64":"aGVsbG8=","filename":"hi.txt"}}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST attach-only = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	h.fake.mu.Lock()
	got := h.fake.last.Attachments
	body := h.fake.last.Body
	h.fake.mu.Unlock()
	if len(got) != 1 {
		t.Errorf("sender attachments = %v, want one staged path", got)
	}
	if body != "" {
		t.Errorf("sender body = %q, want empty", body)
	}
}

func TestNotifyTagQueryFallbacks(t *testing.T) {
	h := newNotifyHarness(config.Config{})
	rec := doPost(h, "/notify?tag=family:2", "application/x-www-form-urlencoded",
		url.Values{"urls": {"json://localhost"}, "body": {"hi"}}.Encode(), nil)
	if rec.Code != http.StatusOK {
		t.Errorf("?tag= = %d, want 200", rec.Code)
	}
	h.fake.mu.Lock()
	tag := h.fake.last.Tag
	h.fake.mu.Unlock()
	if len(tag) != 1 || len(tag[0]) != 1 || tag[0][0] != "family:2" {
		t.Errorf("tag = %v, want [[family:2]]", tag)
	}
}
