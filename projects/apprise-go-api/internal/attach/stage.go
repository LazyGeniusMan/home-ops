package attach

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// maxFilenameLen caps attachment filenames, mirroring Python's 250-char cap.
const maxFilenameLen = 250

// defaultFetchTimeout bounds remote attachment downloads.
const defaultFetchTimeout = 30 * time.Second

// Incoming is one multipart file part offered for staging. Any form field
// name is accepted, mirroring Python's request.FILES handling.
type Incoming struct {
	// Field is the form field name the part arrived under.
	Field string
	// Filename is the client-supplied filename; blank or whitespace-only
	// falls back to attachment.NNN.
	Filename string
	// ContentType is the wire Content-Type; "application/octet-stream"
	// (any case) is nulled so the type is guessed from the filename.
	ContentType string
	// Size is the part size in bytes when known, -1 otherwise.
	Size int64
	// Open provides the part content stream. A nil Open is a 400.
	Open func() (io.ReadCloser, error)
}

// Staged is one fully staged, request-scoped attachment backed by a temp
// file. Files live under the configured attach dir and must be removed at
// request end via CleanupAll (defer-remove, never GC-dependent).
type Staged struct {
	Attachment
	// MIME is the resolved content type: wire type when meaningful,
	// otherwise guessed from the filename. Informational — apprise-go
	// re-detects from the staged file on send.
	MIME string
}

// Stager stages one request's attachments with policy enforcement.
type Stager struct {
	limits Limits
	policy *Policy
	client *http.Client
}

// NewStager builds a Stager from lim. An empty AllowURL defaults to "*"
// (Python's APPRISE_ATTACH_ALLOW_URL default); an empty RejectURL disables
// denials. A non-positive FetchTimeout defaults to 30s.
func NewStager(lim Limits) *Stager {
	allow := lim.AllowURL
	if strings.TrimSpace(allow) == "" {
		allow = "*"
	}
	timeout := lim.FetchTimeout
	if timeout <= 0 {
		timeout = defaultFetchTimeout
	}
	policy := NewPolicy(allow, lim.RejectURL)
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if !policy.IsAllowed(req.URL.String()) {
				return Denied(req.URL.String())
			}
			return nil
		},
	}
	return &Stager{limits: lim, policy: policy, client: client}
}

// StageRequest stages a request's attachments (payload entries + multipart
// parts in order; 1-based attachment.NNN numbering). Disabled/over-count/
// over-size content is a 400.
func (s *Stager) StageRequest(payload any, files []Incoming) ([]Staged, error) {
	entries, scalar := normalizePayload(payload)
	count := len(entries) + len(files)
	if scalar && len(entries) == 0 {
		// Top-level garbage (numbers, bools, ...) is ignored, like Python.
		// An empty payload with no files stages nothing.
		if len(files) == 0 {
			return nil, nil
		}
	}
	if s.limits.SizeMB <= 0 {
		if count == 0 {
			return nil, nil
		}
		return nil, Disabled()
	}
	if s.limits.MaxCount > 0 && count > s.limits.MaxCount {
		return nil, TooMany(count, s.limits.MaxCount)
	}
	var staged []Staged
	for _, e := range entries {
		st, skip, err := s.stageEntry(e)
		if err != nil {
			s.cleanupAll(staged)
			return nil, err
		}
		if skip {
			continue
		}
		staged = append(staged, st)
	}
	for _, f := range files {
		no := len(staged) + 1
		st, err := s.stageFile(f, no)
		if err != nil {
			s.cleanupAll(staged)
			return nil, err
		}
		staged = append(staged, st)
	}
	return staged, nil
}

// cleanupAll removes staged files after a staging failure so partial
// results never leak. Idempotent per-file.
func (s *Stager) cleanupAll(staged []Staged) {
	for _, st := range staged {
		if st.Cleanup != nil {
			st.Cleanup()
		}
	}
}

// entry is one normalized payload item with its 1-based position.
type entry struct {
	no  int
	raw any
}

// normalizePayload converts the decoded attachment value into ordered
// entries. It reports scalar=true when the payload was a single scalar or
// garbage value (as opposed to a list), so callers can distinguish "no
// attachments" from "ignored garbage".
func normalizePayload(payload any) ([]entry, bool) {
	switch v := payload.(type) {
	case nil:
		return nil, false
	case string:
		return []entry{{no: 1, raw: v}}, true
	case []byte:
		return []entry{{no: 1, raw: v}}, true
	case map[string]any:
		if len(v) == 0 {
			return nil, true
		}
		return []entry{{no: 1, raw: v}}, true
	case []any:
		out := make([]entry, 0, len(v))
		for i, e := range v {
			out = append(out, entry{no: i + 1, raw: e})
		}
		return out, false
	case []string:
		out := make([]entry, 0, len(v))
		for i, e := range v {
			out = append(out, entry{no: i + 1, raw: e})
		}
		return out, false
	default:
		// Invalid as list entries but ignored as a top-level payload.
		return nil, true
	}
}

// stageEntry stages one payload entry; skip=true means a blank string that
// is ignored without error.
func (s *Stager) stageEntry(e entry) (st Staged, skip bool, err error) {
	fallback := fmt.Sprintf("attachment.%03d", e.no)
	switch v := e.raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return Staged{}, true, nil
		}
		return s.stageRemote(v, "", fallback)
	case []byte:
		return s.stageBytes(fallback, v)
	case map[string]any:
		return s.stageDict(v, fallback, e.no)
	default:
		return Staged{}, false, BadAttachment("an invalid filename was provided for attachment %d", e.no)
	}
}

// stageDict stages a {base64,filename?} or {url,filename?} dict. The dict
// filename wins over URL-derived names; bad base64, over-long or
// non-string filenames, and dicts with neither key are 400s.
func (s *Stager) stageDict(m map[string]any, fallback string, no int) (Staged, bool, error) {
	name := fallback
	if raw, ok := m["filename"]; ok {
		str, ok := raw.(string)
		if !ok {
			return Staged{}, false, BadAttachment("an invalid filename was provided for attachment %d", no)
		}
		str = strings.TrimSpace(str)
		if len(str) > maxFilenameLen {
			return Staged{}, false, BadAttachment("the filename associated with attachment %d is too long", no)
		}
		if str != "" {
			name = str
		}
	}
	if raw, ok := m["base64"]; ok {
		str, ok := raw.(string)
		if !ok {
			return Staged{}, false, BadAttachment("invalid filecontent was provided for attachment %q", name)
		}
		data, err := decodeBase64(str)
		if err != nil {
			return Staged{}, false, BadAttachment("invalid filecontent was provided for attachment %q", name)
		}
		st, skip, err := s.stageBytes(name, data)
		return st, skip, err
	}
	if raw, ok := m["url"]; ok {
		str, ok := raw.(string)
		if !ok || strings.TrimSpace(str) == "" {
			return Staged{}, false, BadAttachment("invalid filetype was provided for attachment %q", name)
		}
		explicit := ""
		if name != fallback {
			explicit = name
		}
		return s.stageRemote(str, explicit, fallback)
	}
	return Staged{}, false, BadAttachment("invalid filetype was provided for attachment %q", name)
}

// stageRemote validates, SSRF-checks, downloads, and stages a remote URL
// (explicit dict filename wins; fallback is attachment.NNN).
func (s *Stager) stageRemote(rawURL, explicit, fallback string) (Staged, bool, error) {
	trimmed := strings.TrimSpace(rawURL)
	if !isWebURL(trimmed) {
		return Staged{}, false, BadAttachment("failed to load attachment (not web request): %s", redactURL(rawURL))
	}
	if !s.policy.IsAllowed(trimmed) {
		return Staged{}, false, Denied(trimmed)
	}
	name := explicit
	if name == "" {
		name = remoteName(trimmed, fallback)
	}
	if len(name) > maxFilenameLen {
		return Staged{}, false, BadAttachment("the filename associated with attachment %q is too long", name)
	}
	maxBytes := s.limits.SizeMB * 1024 * 1024
	body, mimeType, err := s.fetch(trimmed, name, maxBytes)
	if err != nil {
		return Staged{}, false, err
	}
	defer func() { _ = body.Close() }()
	if mimeType == "" {
		mimeType = resolveMIME("", name)
	}
	st, err := s.stageStream(name, mimeType, body, maxBytes)
	return st, false, err
}

// stageFile stages one multipart part under the per-file cap.
func (s *Stager) stageFile(f Incoming, no int) (Staged, error) {
	fallback := fmt.Sprintf("attachment.%03d", no)
	name := strings.TrimSpace(f.Filename)
	if len(f.Filename) > maxFilenameLen || len(name) > maxFilenameLen {
		return Staged{}, BadAttachment("the filename associated with attachment %d is too long", no)
	}
	if name == "" {
		name = fallback
	}
	if f.Open == nil {
		return Staged{}, BadAttachment("an invalid filename was provided for attachment %d", no)
	}
	wire := strings.ToLower(strings.TrimSpace(f.ContentType))
	if wire == "application/octet-stream" {
		wire = ""
	}
	rc, err := f.Open()
	if err != nil {
		return Staged{}, BadAttachment("could not read attachment %q: %w", name, err)
	}
	defer func() { _ = rc.Close() }()
	maxBytes := s.limits.SizeMB * 1024 * 1024
	return s.stageStream(name, resolveMIME(wire, name), rc, maxBytes)
}

// stageBytes stages small inline content under the per-file cap.
func (s *Stager) stageBytes(name string, data []byte) (Staged, bool, error) {
	if len(name) > maxFilenameLen {
		return Staged{}, false, BadAttachment("attachment name too long: %q", name)
	}
	maxBytes := s.limits.SizeMB * 1024 * 1024
	if int64(len(data)) > maxBytes {
		return Staged{}, false, TooLargeFile(name, maxBytes)
	}
	st, err := StageTemp(s.limits.Dir, name, data)
	if err != nil {
		return Staged{}, false, err
	}
	return Staged{Attachment: st, MIME: resolveMIME("", name)}, false, nil
}

// stageStream streams r to a request-scoped tempfile, enforcing maxBytes.
// Over-cap content aborts the stage and removes the partial file.
func (s *Stager) stageStream(name, mimeType string, r io.Reader, maxBytes int64) (Staged, error) {
	if name == "" {
		return Staged{}, BadAttachment("blank attachment name")
	}
	if len(name) > maxFilenameLen {
		return Staged{}, BadAttachment("attachment name too long: %q", name)
	}
	dir := s.limits.Dir
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Staged{}, BadAttachment("could not create directory %s", dir)
	}
	f, err := os.CreateTemp(dir, "apprise-attach-*")
	if err != nil {
		return Staged{}, BadAttachment("could not prepare %s attachment in %s", name, dir)
	}
	tmp := f.Name()
	n, err := io.Copy(f, io.LimitReader(r, maxBytes+1))
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return Staged{}, BadAttachment("could not write attachment %s to disk", name)
	}
	if n > maxBytes {
		_ = os.Remove(tmp)
		return Staged{}, TooLargeFile(name, maxBytes)
	}
	cleanup := func() { _ = os.Remove(tmp) }
	return Staged{Attachment: Attachment{Path: tmp, Name: name, Cleanup: cleanup}, MIME: mimeType}, nil
}

// fetch downloads a remote attachment after an early Content-Length
// fast-fail. Non-2xx statuses and network errors are FetchFailed (400).
func (s *Stager) fetch(rawURL, name string, maxBytes int64) (io.ReadCloser, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", FetchFailed(rawURL, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		var denied *StatusError
		if errors.As(err, &denied) && denied.Code == StatusBadRequest {
			return nil, "", err
		}
		return nil, "", FetchFailed(rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, "", FetchFailed(rawURL, fmt.Errorf("unexpected status %d", resp.StatusCode))
	}
	if resp.ContentLength > maxBytes {
		_ = resp.Body.Close()
		return nil, "", TooLargeFile(name, maxBytes)
	}
	mimeType := ""
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mimeType = strings.ToLower(strings.TrimSpace(ct))
		if i := strings.Index(mimeType, ";"); i >= 0 {
			mimeType = strings.TrimSpace(mimeType[:i])
		}
		if mimeType == "application/octet-stream" {
			mimeType = ""
		}
	}
	return resp.Body, mimeType, nil
}

// isWebURL reports whether raw is an http(s) URL with content after the
// scheme, mirroring Python's ^https?://.+ check.
func isWebURL(raw string) bool {
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"http://", "https://"} {
		if strings.HasPrefix(lower, prefix) && len(raw) > len(prefix) {
			return true
		}
	}
	return false
}

// remoteName resolves a remote attachment filename: ?name= (sanitized to
// its basename, ignored when empty) wins, then the URL path basename,
// then the attachment.NNN fallback.
func remoteName(rawURL, fallback string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fallback
	}
	if q := strings.TrimSpace(parsed.Query().Get("name")); q != "" {
		// Basename-only so ?name=/etc/passwd or ../../secret.jpg cannot
		// smuggle paths; Name only labels the notification anyway.
		base := path.Base(strings.ReplaceAll(q, "\\", "/"))
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	if base := path.Base(strings.TrimSuffix(parsed.Path, "/")); base != "" && base != "." && base != "/" {
		return base
	}
	return fallback
}

// decodeBase64 strictly decodes base64 content, ignoring ASCII whitespace
// (which Python's b64decode tolerates). Non-alphabet input is a 400.
func decodeBase64(raw string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		default:
			return r
		}
	}, raw)
	return base64.StdEncoding.DecodeString(clean)
}

// resolveMIME picks the attachment content type: the wire type when
// meaningful, otherwise guessed from the filename extension, defaulting to
// application/octet-stream. Mixed-case wire types are normalized.
func resolveMIME(wire, name string) string {
	if wire != "" {
		return wire
	}
	if ext := strings.ToLower(path.Ext(name)); ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			if media, _, err := mime.ParseMediaType(mt); err == nil && media != "" {
				return media
			}
			if i := strings.Index(mt, ";"); i >= 0 {
				return strings.TrimSpace(mt[:i])
			}
			return mt
		}
	}
	return "application/octet-stream"
}
