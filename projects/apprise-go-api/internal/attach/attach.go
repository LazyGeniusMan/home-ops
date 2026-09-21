// Package attach stages stateless notification attachments into
// request-scoped temp files.
//
// G3 implements the full mechanisms (multipart parts, remote http(s) URLs,
// JSON {base64,filename}/{url,filename} dicts), limits (APPRISE_ATTACH_SIZE,
// APPRISE_MAX_ATTACHMENTS, APPRISE_UPLOAD_MAX_MEMORY_SIZE), the SSRF
// allow/reject policy, and cleanup. Attachments never persist: staged files
// live under APPRISE_ATTACH_DIR (or os.TempDir) and are removed when the
// request ends.
package attach

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Limits carries the G3 attachment knobs: APPRISE_ATTACH_DIR,
// APPRISE_ATTACH_SIZE, APPRISE_MAX_ATTACHMENTS, the SSRF allow/reject
// lists, and the remote-fetch timeout. See config.AttachLimits.
type Limits struct {
	// Dir stages temp files; empty means os.TempDir().
	Dir string
	// SizeMB is the per-file limit in MiB; <=0 disables attachments.
	SizeMB int64
	// MaxCount caps attachments per request; 0 = unlimited.
	MaxCount int
	// AllowURL is the SSRF allowlist (APPRISE_ATTACH_ALLOW_URL);
	// empty means "*" (Python's default).
	AllowURL string
	// RejectURL is the SSRF denylist (APPRISE_ATTACH_REJECT_URL);
	// empty disables denials.
	RejectURL string
	// FetchTimeout bounds remote attachment downloads; <=0 means 30s.
	FetchTimeout time.Duration
}

// Attachment is a staged, request-scoped attachment file.
type Attachment struct {
	// Path is the staged temp file backing the attachment.
	Path string
	// Name is the attachment filename (<=250 chars, never blank).
	Name string
	// Cleanup removes the staged file. Idempotent.
	Cleanup func()
}

// Stage validates count/disabled policy via the full Stager, treating each
// name as a remote-URL-or-inline payload entry. Kept compatible for the G2
// seam (server.checkAttachmentsStub): count/disabled errors surface as
// StatusError values, and any non-empty name proceeds to real staging, so
// callers see either a policy error or staged (possibly remote-fetched)
// files. maxMemoryBytes must be positive (the multipart/memory budget the
// caller enforces via MaxBytesReader before staging).
func Stage(names []string, lim Limits, maxMemoryBytes int64) ([]Attachment, error) {
	if maxMemoryBytes <= 0 {
		return nil, BadAttachment("max memory bytes must be positive, got %d", maxMemoryBytes)
	}
	stager := NewStager(lim)
	entries := make([]any, 0, len(names))
	for _, n := range names {
		entries = append(entries, n)
	}
	staged, err := stager.StageRequest(entries, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Attachment, 0, len(staged))
	for _, st := range staged {
		out = append(out, st.Attachment)
	}
	return out, nil
}

// CleanupAll removes every staged file. Idempotent per file; safe to defer
// at request end for zero persistence (never GC-dependent).
func CleanupAll(staged []Staged) {
	for _, st := range staged {
		if st.Cleanup != nil {
			st.Cleanup()
		}
	}
}

// Paths returns the staged local paths for apprise-go WithAttachments.
// Local temp paths are passed as-is: apprise-go reads them as files and
// never fetches them, so the SSRF policy (already enforced here) holds.
func Paths(staged []Staged) []string {
	out := make([]string, 0, len(staged))
	for _, st := range staged {
		out = append(out, st.Path)
	}
	return out
}

// Names returns the attachment filenames in staged order.
func Names(staged []Staged) []string {
	out := make([]string, 0, len(staged))
	for _, st := range staged {
		out = append(out, st.Name)
	}
	return out
}

// HasAttachment reports whether a request carries attachment content:
// staged files or a non-blank payload. G2 uses this for the
// body-not-required-when-attachment rule.
func HasAttachment(staged []Staged, payload any) bool {
	if len(staged) > 0 {
		return true
	}
	entries, _ := normalizePayload(payload)
	for _, e := range entries {
		if s, ok := e.raw.(string); ok {
			// Blank strings are ignored entries (Python decrements and
			// moves along), so they do not satisfy the body rule.
			if strings.TrimSpace(s) != "" {
				return true
			}
			continue
		}
		if e.raw != nil {
			return true
		}
	}
	return false
}

// StageTemp creates a request-scoped temp file under dir (or os.TempDir
// when dir is empty) and returns an Attachment whose Cleanup removes it.
func StageTemp(dir, name string, data []byte) (Attachment, error) {
	if name == "" {
		return Attachment{}, fmt.Errorf("attach: blank attachment name")
	}
	if len(name) > 250 {
		return Attachment{}, fmt.Errorf("attach: attachment name too long: %q", name)
	}
	base := dir
	if base == "" {
		base = os.TempDir()
	}
	f, err := os.CreateTemp(base, "apprise-attach-*")
	if err != nil {
		return Attachment{}, fmt.Errorf("attach: create temp file: %w", err)
	}
	tmpName := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpName)
		return Attachment{}, fmt.Errorf("attach: write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpName)
		return Attachment{}, fmt.Errorf("attach: close temp file: %w", err)
	}
	// Keep the unique temp path to avoid collisions; Name carries the
	// original filename for the notification.
	cleanup := func() { _ = os.Remove(tmpName) }
	return Attachment{Path: tmpName, Name: name, Cleanup: cleanup}, nil
}
