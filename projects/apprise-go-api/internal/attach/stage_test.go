package attach

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLimits(dir string) Limits {
	return Limits{Dir: dir, SizeMB: 200, MaxCount: 6, AllowURL: "*", RejectURL: ""}
}

func mustStage(t *testing.T, s *Stager, payload any, files []Incoming) []Staged {
	t.Helper()
	staged, err := s.StageRequest(payload, files)
	if err != nil {
		t.Fatalf("StageRequest() = %v", err)
	}
	t.Cleanup(func() { CleanupAll(staged) })
	return staged
}

func readStaged(t *testing.T, st Staged) string {
	t.Helper()
	data, err := os.ReadFile(st.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v", st.Path, err)
	}
	return string(data)
}

func incomingFile(field, name, contentType, body string) Incoming {
	return Incoming{
		Field:       field,
		Filename:    name,
		ContentType: contentType,
		Size:        int64(len(body)),
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(body)), nil
		},
	}
}

func TestStageRequestEmpty(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	for _, payload := range []any{nil, []any{}, map[string]any(nil)} {
		got, err := s.StageRequest(payload, nil)
		if err != nil {
			t.Fatalf("StageRequest(%v) = %v", payload, err)
		}
		if len(got) != 0 {
			t.Errorf("StageRequest(%v) = %d, want 0", payload, len(got))
		}
	}
}

func TestStageRequestTopLevelGarbageIgnored(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	for _, payload := range []any{5, 5.5, true} {
		got, err := s.StageRequest(payload, nil)
		if err != nil {
			t.Fatalf("StageRequest(%v) = %v", payload, err)
		}
		if len(got) != 0 {
			t.Errorf("StageRequest(%v) = %d, want 0", payload, len(got))
		}
	}
	// A set-like blank entry is ignored too.
	got, err := s.StageRequest("", nil)
	if err != nil {
		t.Fatalf("StageRequest(blank) = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("StageRequest(blank) = %d, want 0", len(got))
	}
}

func TestStageRequestDisabled(t *testing.T) {
	s := NewStager(Limits{Dir: t.TempDir(), SizeMB: 0})
	if _, err := s.StageRequest(nil, nil); err != nil {
		t.Errorf("StageRequest(disabled, empty) = %v, want nil", err)
	}
	payload := map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("x"))}
	if _, err := s.StageRequest(payload, nil); err == nil {
		t.Error("StageRequest(disabled, payload) = nil, want error")
	} else if StatusCodeOf(err) != StatusBadRequest {
		t.Errorf("StatusCodeOf() = %d, want 400", StatusCodeOf(err))
	}
	if _, err := s.StageRequest(nil, []Incoming{incomingFile("f", "a.txt", "text/plain", "x")}); err == nil {
		t.Error("StageRequest(disabled, files) = nil, want error")
	}
}

func TestStageRequestMaxCount(t *testing.T) {
	dir := t.TempDir()
	mk := func() Limits { return Limits{Dir: dir, SizeMB: 200, MaxCount: 3, AllowURL: "*"} }
	payload := []any{
		map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("one"))},
		map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("two"))},
	}
	files := []Incoming{
		incomingFile("attachment", "a.txt", "text/plain", "a"),
		incomingFile("attachment", "b.txt", "text/plain", "b"),
	}
	// 2 payload + 2 files = 4 > 3.
	if _, err := NewStager(mk()).StageRequest(payload, files); err == nil {
		t.Error("StageRequest(over max) = nil, want error")
	}
	// 0 = unlimited.
	unlimited := mk()
	unlimited.MaxCount = 0
	if _, err := NewStager(unlimited).StageRequest(payload, files); err != nil {
		t.Errorf("StageRequest(unlimited) = %v, want nil", err)
	}
}

func TestStageRequestBase64Dict(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	staged := mustStage(t, s, map[string]any{
		"base64":   base64.StdEncoding.EncodeToString([]byte("data to be encoded")),
		"filename": "myfile.bin",
	}, nil)
	if len(staged) != 1 {
		t.Fatalf("len() = %d, want 1", len(staged))
	}
	if staged[0].Name != "myfile.bin" {
		t.Errorf("Name = %q, want myfile.bin", staged[0].Name)
	}
	if got := readStaged(t, staged[0]); got != "data to be encoded" {
		t.Errorf("content = %q, want decoded payload", got)
	}
}

func TestStageRequestBase64FilenameVariants(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	// Whitespace filename falls back to attachment.001.
	staged := mustStage(t, s, map[string]any{
		"base64":   base64.StdEncoding.EncodeToString([]byte("x")),
		"filename": "   ",
	}, nil)
	if staged[0].Name != "attachment.001" {
		t.Errorf("Name = %q, want attachment.001", staged[0].Name)
	}
	// Over-long filename is a 400.
	if _, err := s.StageRequest(map[string]any{
		"base64":   base64.StdEncoding.EncodeToString([]byte("x")),
		"filename": strings.Repeat("a", 1000),
	}, nil); err == nil {
		t.Error("long filename = nil, want error")
	}
	// Non-string filenames are 400s.
	for _, bad := range []any{1, nil, []string{"x"}} {
		if _, err := s.StageRequest(map[string]any{
			"base64":   base64.StdEncoding.EncodeToString([]byte("x")),
			"filename": bad,
		}, nil); err == nil {
			t.Errorf("filename %v = nil, want error", bad)
		}
	}
	// Bad base64 is a 400.
	if _, err := s.StageRequest(map[string]any{"base64": "not-base-64!!!"}, nil); err == nil {
		t.Error("bad base64 = nil, want error")
	}
	// Empty dict is a 400.
	if _, err := s.StageRequest([]any{map[string]any{}}, nil); err == nil {
		t.Error("empty dict = nil, want error")
	}
	// Nil list entry is a 400.
	if _, err := s.StageRequest([]any{nil}, nil); err == nil {
		t.Error("nil entry = nil, want error")
	}
}

func TestStageRequestMultipartFiles(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	// Any field name is accepted.
	files := []Incoming{
		incomingFile("attach1", "a.txt", "text/plain", "a"),
		incomingFile("attach2", "b.txt", "text/plain", "b"),
	}
	staged := mustStage(t, s, nil, files)
	if len(staged) != 2 {
		t.Fatalf("len() = %d, want 2", len(staged))
	}
	if staged[0].Name != "a.txt" || staged[1].Name != "b.txt" {
		t.Errorf("names = %q, want [a.txt b.txt]", Names(staged))
	}
	// Blank filename falls back to attachment.NNN.
	staged = mustStage(t, s, nil, []Incoming{incomingFile("file1", "    ", "text/plain", "content here")})
	if staged[0].Name != "attachment.001" {
		t.Errorf("Name = %q, want attachment.001", staged[0].Name)
	}
	// Combined payload + files count as one request.
	staged = mustStage(t, s,
		map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("x"))},
		[]Incoming{
			incomingFile("attachment", "a.txt", "text/plain", "a"),
			incomingFile("attachment", "b.txt", "text/plain", "b"),
		})
	if len(staged) != 3 {
		t.Errorf("len() = %d, want 3 (1 payload + 2 files)", len(staged))
	}
}

func TestStageRequestWireContentType(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	staged := mustStage(t, s, nil, []Incoming{incomingFile("f", "attachment.001", "image/jpeg", "\xff\xd8\xff\xe0")})
	if staged[0].MIME != "image/jpeg" {
		t.Errorf("MIME = %q, want image/jpeg", staged[0].MIME)
	}
	// application/octet-stream (any case) is nulled so the type is guessed.
	for _, ct := range []string{"application/octet-stream", "APPLICATION/OCTET-STREAM"} {
		staged := mustStage(t, s, nil, []Incoming{incomingFile("f", "poster.jpg", ct, "\xff\xd8\xff\xe0")})
		if staged[0].MIME != "image/jpeg" {
			t.Errorf("MIME(%s) = %q, want image/jpeg", ct, staged[0].MIME)
		}
	}
	// Mixed-case wire types are normalized.
	staged = mustStage(t, s, nil, []Incoming{incomingFile("f", "attachment.001", "Image/JPEG", "\xff\xd8\xff\xe0")})
	if staged[0].MIME != "image/jpeg" {
		t.Errorf("MIME = %q, want image/jpeg", staged[0].MIME)
	}
}

func TestStageRequestSizeLimit(t *testing.T) {
	dir := t.TempDir()
	s := NewStager(Limits{Dir: dir, SizeMB: 1, AllowURL: "*"})
	big := strings.Repeat("content", 1024*1024) // ~7MB
	if _, err := s.StageRequest(nil, []Incoming{incomingFile("f", "big.txt", "text/plain", big)}); err == nil {
		t.Error("oversize file = nil, want error")
	} else if StatusCodeOf(err) != StatusBadRequest {
		t.Errorf("StatusCodeOf() = %d, want 400", StatusCodeOf(err))
	}
	// Entries left on disk after a failure are removed.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "apprise-attach-") {
			t.Errorf("leaked temp file %s after failed stage", e.Name())
		}
	}
}

func TestStageRequestStreamNotBuffered(t *testing.T) {
	// A 5MB part under a 200MB cap stages via streaming; the test only
	// asserts correctness + cleanup, not RSS.
	s := NewStager(testLimits(t.TempDir()))
	big := strings.Repeat("x", 5<<20)
	staged := mustStage(t, s, nil, []Incoming{incomingFile("f", "big.bin", "application/octet-stream", big)})
	if got := readStaged(t, staged[0]); len(got) != 5<<20 {
		t.Errorf("len() = %d, want %d", len(got), 5<<20)
	}
}

func TestStageRemoteStringURLs(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := NewStager(testLimits(t.TempDir()))

	body = "png-bytes"
	staged := mustStage(t, s, srv.URL+"/myfile.png", nil)
	if len(staged) != 1 {
		t.Fatalf("len() = %d, want 1", len(staged))
	}
	if staged[0].Name != "myfile.png" {
		t.Errorf("Name = %q, want myfile.png (path basename)", staged[0].Name)
	}
	if got := readStaged(t, staged[0]); got != "png-bytes" {
		t.Errorf("content = %q, want png-bytes", got)
	}

	// Local files and non-URLs are 400s.
	for _, bad := range []string{"file:///etc/hosts", "/etc/hosts", "simply invalid"} {
		if _, err := s.StageRequest(bad, nil); err == nil {
			t.Errorf("StageRequest(%q) = nil, want error", bad)
		} else if StatusCodeOf(err) != StatusBadRequest {
			t.Errorf("StatusCodeOf(%q) = %d, want 400", bad, StatusCodeOf(err))
		}
	}
}

func TestStageRemoteNameResolution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()
	s := NewStager(testLimits(t.TempDir()))

	// ?name= wins.
	staged := mustStage(t, s, srv.URL+"/thumbnails/6dba.jpg?name=thumbnail.jpg", nil)
	if staged[0].Name != "thumbnail.jpg" {
		t.Errorf("Name = %q, want thumbnail.jpg", staged[0].Name)
	}
	// No ?name=: path basename is used.
	staged = mustStage(t, s, srv.URL+"/thumbnails/photo.jpg", nil)
	if staged[0].Name != "photo.jpg" {
		t.Errorf("Name = %q, want photo.jpg", staged[0].Name)
	}
	// No path filename: attachment.NNN fallback.
	staged = mustStage(t, s, srv.URL+"/", nil)
	if staged[0].Name != "attachment.001" {
		t.Errorf("Name = %q, want attachment.001", staged[0].Name)
	}
	// Empty/whitespace ?name=: ignored.
	staged = mustStage(t, s, srv.URL+"/thumbnails/6dba.jpg?name=", nil)
	if staged[0].Name != "6dba.jpg" {
		t.Errorf("Name = %q, want 6dba.jpg", staged[0].Name)
	}
	// Traversal in ?name=: basename only.
	staged = mustStage(t, s, srv.URL+"/x.jpg?name=/etc/passwd", nil)
	if staged[0].Name != "passwd" {
		t.Errorf("Name = %q, want passwd", staged[0].Name)
	}
	staged = mustStage(t, s, srv.URL+"/x.jpg?name=../../secret.jpg", nil)
	if staged[0].Name != "secret.jpg" {
		t.Errorf("Name = %q, want secret.jpg", staged[0].Name)
	}
}

func TestStageDictURLNamePriority(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()
	s := NewStager(testLimits(t.TempDir()))

	// Dict filename beats URL ?name=.
	staged := mustStage(t, s, []any{map[string]any{
		"url":      srv.URL + "/img.jpg?name=url_name.jpg",
		"filename": "custom.jpg",
	}}, nil)
	if staged[0].Name != "custom.jpg" {
		t.Errorf("Name = %q, want custom.jpg", staged[0].Name)
	}
	// URL ?name= used when no dict filename.
	staged = mustStage(t, s, []any{map[string]any{
		"url": srv.URL + "/img.jpg?name=thumbnail.jpg",
	}}, nil)
	if staged[0].Name != "thumbnail.jpg" {
		t.Errorf("Name = %q, want thumbnail.jpg", staged[0].Name)
	}
	// Path basename auto-detect.
	staged = mustStage(t, s, []any{map[string]any{"url": srv.URL + "/thumbnails/photo.jpg"}}, nil)
	if staged[0].Name != "photo.jpg" {
		t.Errorf("Name = %q, want photo.jpg", staged[0].Name)
	}
	// No path filename: attachment.NNN.
	staged = mustStage(t, s, []any{map[string]any{"url": srv.URL + "/"}}, nil)
	if staged[0].Name != "attachment.001" {
		t.Errorf("Name = %q, want attachment.001", staged[0].Name)
	}
	// Traversal: basename only.
	staged = mustStage(t, s, []any{map[string]any{"url": srv.URL + "/img.jpg?name=/etc/passwd"}}, nil)
	if staged[0].Name != "passwd" {
		t.Errorf("Name = %q, want passwd", staged[0].Name)
	}
}

func TestStageRemoteDenied(t *testing.T) {
	s := NewStager(Limits{Dir: t.TempDir(), SizeMB: 200, AllowURL: "*", RejectURL: "*"})
	if _, err := s.StageRequest("http://example.com/x.png", nil); err == nil {
		t.Error("deny * = nil, want error")
	} else if StatusCodeOf(err) != StatusBadRequest {
		t.Errorf("StatusCodeOf() = %d, want 400", StatusCodeOf(err))
	}
}

func TestStageRemoteFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	s := NewStager(testLimits(t.TempDir()))
	if _, err := s.StageRequest(srv.URL+"/missing.png", nil); err == nil {
		t.Error("404 fetch = nil, want error")
	}
}

func TestPolicyDenyFirst(t *testing.T) {
	p := NewPolicy("example.com", "example.com")
	if p.IsAllowed("http://example.com/x") {
		t.Error("deny+allow conflict: want denied (DENY-first)")
	}
	p = NewPolicy("127.0.* localhost*", "")
	// Localhost blocked by allow-list miss... allow covers it, no deny.
	if !p.IsAllowed("http://localhost/x") {
		t.Error("localhost with allow 127.0.* localhost*: want allowed")
	}
	p = NewPolicy("*", "127.0.* localhost*")
	if p.IsAllowed("http://127.0.0.1/x") || p.IsAllowed("http://localhost/x") {
		t.Error("default Python policy: want loopback denied")
	}
	if !p.IsAllowed("http://example.com/x") {
		t.Error("default Python policy: want public host allowed")
	}
	if p.IsAllowed("file:///etc/hosts") || p.IsAllowed("/etc/hosts") || p.IsAllowed("simply invalid") {
		t.Error("non-URL: want denied")
	}
}

func TestPolicyInternalToken(t *testing.T) {
	old := resolveHost
	defer func() { resolveHost = old }()
	resolveHost = func(host string) ([]netip.Addr, error) {
		// Literal IPs never reach DNS (resolveHost short-circuits them),
		// so only stub hostname lookups here.
		if host == "localhost" || host == "localhost.localdomain" {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		if addr, err := parseIPLiteral(host); err == nil {
			return []netip.Addr{addr}, nil
		}
		return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
	}
	p := NewPolicy("*", "internal")
	for _, raw := range []string{
		"http://127.0.0.1/x",
		"http://2130706433/x", // decimal 127.0.0.1
		"http://[::1]/x",
		"http://[::ffff:127.0.0.1]/x",
		"http://10.0.0.5/x",
		"http://172.16.0.5/x",
		"http://192.168.1.1/x",
		"http://169.254.169.254/x",
		"http://0.0.0.0/x",
		"http://224.0.0.1/x",
		"http://100.64.0.1/x",
		"http://localhost/x",
	} {
		if p.IsAllowed(raw) {
			t.Errorf("IsAllowed(%q) = true, want false", raw)
		}
	}
	if !p.IsAllowed("http://8.8.8.8/x") {
		t.Error("IsAllowed(public IP) = false, want true")
	}
	if !p.IsAllowed("http://example.com/x") {
		t.Error("IsAllowed(public DNS) = false, want true")
	}
	// Unresolvable host fails closed.
	resolveHost = func(string) ([]netip.Addr, error) { return nil, io.ErrUnexpectedEOF }
	if p.IsAllowed("http://this-does-not-resolve.invalid/x") {
		t.Error("unresolvable host: want denied")
	}
	// internal in the allow list is a no-op (nothing positively matches).
	p = NewPolicy("internal", "")
	if p.IsAllowed("http://8.8.8.8/x") || p.IsAllowed("http://example.com/x") {
		t.Error("allow=internal: want denied")
	}
}

func TestPolicyWildcardSemantics(t *testing.T) {
	p := NewPolicy("localhost?", "")
	if p.IsAllowed("http://localhost/x") {
		t.Error("localhost? should not match localhost")
	}
	if !p.IsAllowed("http://localhost1/x") {
		t.Error("localhost? should match localhost1")
	}
	// Scheme pin: http-only rule rejects https.
	p = NewPolicy("http://example.com", "")
	if !p.IsAllowed("http://example.com/x") {
		t.Error("http rule: want http allowed")
	}
	if p.IsAllowed("https://example.com/x") {
		t.Error("http rule: want https denied")
	}
}

func TestCleanupAllRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewStager(testLimits(dir))
	staged := mustStage(t, s, map[string]any{
		"base64": base64.StdEncoding.EncodeToString([]byte("x")),
	}, nil)
	path := staged[0].Path
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat() = %v, want staged file", err)
	}
	CleanupAll(staged)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Stat() after CleanupAll = %v, want not-exist", err)
	}
	CleanupAll(staged) // idempotent
}

func TestHasAttachment(t *testing.T) {
	if HasAttachment(nil, nil) {
		t.Error("HasAttachment(nil) = true, want false")
	}
	if HasAttachment(nil, "") || HasAttachment(nil, "   ") {
		t.Error("HasAttachment(blank) = true, want false")
	}
	if !HasAttachment(nil, "http://example.com/x.png") {
		t.Error("HasAttachment(url) = false, want true")
	}
	staged := []Staged{{Attachment: Attachment{Path: "p", Name: "n"}}}
	if !HasAttachment(staged, nil) {
		t.Error("HasAttachment(staged) = false, want true")
	}
}

func TestPathsAndNames(t *testing.T) {
	s := NewStager(testLimits(t.TempDir()))
	staged := mustStage(t, s, []any{
		map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("1")), "filename": "a.txt"},
		map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("2")), "filename": "b.txt"},
	}, nil)
	if got := Names(staged); len(got) != 2 || got[0] != "a.txt" || got[1] != "b.txt" {
		t.Errorf("Names() = %q, want [a.txt b.txt]", got)
	}
	paths := Paths(staged)
	if len(paths) != 2 {
		t.Fatalf("Paths() len = %d, want 2", len(paths))
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("Paths() = %q, want absolute temp path", p)
		}
	}
}

func TestStatusMapping(t *testing.T) {
	if got := StatusCodeOf(Disabled()); got != StatusBadRequest {
		t.Errorf("Disabled = %d, want 400", got)
	}
	if got := StatusCodeOf(TooMany(7, 6)); got != StatusBadRequest {
		t.Errorf("TooMany = %d, want 400", got)
	}
	if got := StatusCodeOf(BodyTooLarge(3 << 20)); got != StatusFieldsTooLarge {
		t.Errorf("BodyTooLarge = %d, want 431", got)
	}
	if got := StatusCodeOf(WrapSendError("json://x", "a.txt", io.ErrUnexpectedEOF)); got != StatusFailedDependency {
		t.Errorf("SendFailure = %d, want 424", got)
	}
	if got := StatusCodeOf(io.ErrUnexpectedEOF); got != 500 {
		t.Errorf("unknown = %d, want 500", got)
	}
	if !IsUnsupportedAttachments(io.ErrUnexpectedEOF) {
		// sanity: plain errors are not unsupported-attachment errors.
	} else {
		t.Error("IsUnsupportedAttachments(random) = true, want false")
	}
	if !IsUnsupportedAttachments(WrapSendError("json://x", "a.txt", errUnsupportedFixture())) {
		t.Error("IsUnsupportedAttachments(wrapped sentinel) = false, want true")
	}
}

func TestFetchTimeoutBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("slow"))
	}))
	defer srv.Close()
	s := NewStager(Limits{Dir: t.TempDir(), SizeMB: 200, AllowURL: "*", FetchTimeout: 50 * time.Millisecond})
	if _, err := s.StageRequest(srv.URL+"/slow.png", nil); err == nil {
		t.Error("slow fetch = nil, want timeout error")
	}
}
