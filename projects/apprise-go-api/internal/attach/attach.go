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
)

// Limits mirrors the G3 attachment knobs for stub wiring.
type Limits struct {
	// Dir stages temp files; empty means os.TempDir().
	Dir string
	// SizeMB is the per-file limit in MiB; <=0 disables attachments.
	SizeMB int64
	// MaxCount caps attachments per request; 0 = unlimited.
	MaxCount int
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

// Stage validates count/disabled policy and returns no staged files.
// Full staging lands in G3.
func Stage(names []string, lim Limits, maxMemoryBytes int64) ([]Attachment, error) {
	if maxMemoryBytes <= 0 {
		return nil, fmt.Errorf("attach: max memory bytes must be positive, got %d", maxMemoryBytes)
	}
	if lim.SizeMB <= 0 && len(names) > 0 {
		return nil, fmt.Errorf("attach: attachments are disabled")
	}
	if lim.MaxCount > 0 && len(names) > lim.MaxCount {
		return nil, fmt.Errorf("attach: too many attachments: got %d, max %d", len(names), lim.MaxCount)
	}
	return nil, nil
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
