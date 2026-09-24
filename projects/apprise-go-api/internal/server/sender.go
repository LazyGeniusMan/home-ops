// Payload decoding for POST /notify: content-type detection, JSON and form
// parsing, ':' remap application, and attachment staging. serveNotify
// (handler.go) calls decodePayload, then validates the decoded
// notifyRequest (validation.go) before sending.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/attach"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/remap"
)

// senderIface is the notify.Sender contract the handlers depend on. The
// concrete *notify.Sender satisfies it; tests substitute fakes without
// changing production wiring.
type senderIface interface {
	Send(ctx context.Context, req notify.Request) (notify.Result, error)
	Timeout() time.Duration
}

// notifyRequest is the decoded stateless payload: one struct for all three
// content types (JSON, multipart, urlencoded).
type notifyRequest struct {
	// URLs holds the raw urls value: string (comma/space split) or list.
	URLs any
	// Body is the notification body.
	Body string
	// Title is the optional title.
	Title string
	// NotifyType is the raw type value.
	NotifyType string
	// Format is the raw format value.
	Format string
	// Tag holds the raw tag value (string or list straight through).
	Tag any
	// Tags is the raw tags alias (tag wins).
	Tags any
	// Attach holds attachment payloads by alias for the staging gate.
	Attach []string
	// AttachRaw is the winning attach alias value in decoded shape
	// (string, list, or dict) handed to attachment staging.
	AttachRaw any
	// Files holds multipart file parts in arrival order for the stager.
	Files []attach.Incoming
	// HasAttach reports any attach/attachment/attachments alias or file part.
	HasAttach bool
	// FileCount counts uploaded file parts for staging.
	FileCount int
}

// errPayloadTooLarge reports a JSON body over APPRISE_UPLOAD_MAX_MEMORY_SIZE.
var errPayloadTooLarge = fmt.Errorf("notify: JSON payload too large")

// decodePayload parses the request body per content type and returns the
// decoded request, whether it was JSON, the raw field map (for remap), and
// an error. JSON detection uses the content-type regex; everything else is
// treated as form (urlencoded or multipart via r.ParseMultipartForm).
func (s *Server) decodePayload(r *http.Request) (*notifyRequest, bool, map[string]any, error) {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = r.Header.Get("content-type")
	}
	if mimeIsJSON.MatchString(ct) {
		return decodeJSONPayload(r, uploadMaxBytes(s.cfg.UploadMaxMemorySizeMB))
	}
	return decodeFormPayload(r)
}

// decodeJSONPayload decodes a JSON stateless body. Unknown shapes, scalar
// JSON, or empty objects yield nil (→ 400 "Bad FORM Payload"); oversize
// bodies yield errPayloadTooLarge (→ 431).
func decodeJSONPayload(r *http.Request, maxBytes int64) (*notifyRequest, bool, map[string]any, error) {
	if maxBytes <= 0 {
		maxBytes = 3 << 20
	}
	limited := io.LimitReader(r.Body, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, true, nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, true, nil, errPayloadTooLarge
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&doc); err != nil {
		return nil, true, nil, err
	}
	if len(doc) == 0 {
		return nil, true, doc, nil
	}
	out := &notifyRequest{}
	if v, ok := doc["urls"]; ok {
		out.URLs = v
	}
	out.Body, _ = doc["body"].(string)
	out.Title, _ = doc["title"].(string)
	out.NotifyType, _ = doc["type"].(string)
	out.Format, _ = doc["format"].(string)
	out.Tag = doc["tag"]
	out.Tags = doc["tags"]
	// Attach aliases: attach > attachment > attachments; canonicalize the
	// winner into Attach and keep its decoded shape in AttachRaw for
	// staging.
	for _, alias := range []string{"attach", "attachment", "attachments"} {
		v, ok := doc[alias]
		if !ok || v == nil {
			continue
		}
		out.Attach = append(out.Attach, attachStrings(v)...)
		out.AttachRaw = v
		out.HasAttach = true
		break
	}
	return out, true, doc, nil
}

// decodeFormPayload parses urlencoded and multipart forms. Django semantics:
// every value arrives as a string list; first value wins for scalars; the
// winning attach alias collects all its values (blanks dropped). Unknown or
// empty forms yield nil (→ 400 "Bad FORM Payload"). FORM urls over
// urlsMaxLen chars are dropped before validation (→ 204 downstream).
func decodeFormPayload(r *http.Request) (*notifyRequest, bool, map[string]any, error) {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(ct), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil && err != http.ErrNotMultipart {
			return nil, false, nil, err
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return nil, false, nil, err
		}
	}
	if r.PostForm == nil && r.MultipartForm == nil {
		return nil, false, nil, nil
	}
	raw := map[string]any{}
	put := func(k string, vs []string) {
		if len(vs) == 1 {
			raw[k] = vs[0]
		} else {
			cp := make([]any, len(vs))
			for i, v := range vs {
				cp[i] = v
			}
			raw[k] = cp
		}
	}
	if r.PostForm != nil {
		for k, vs := range r.PostForm {
			put(k, vs)
		}
	}
	// Multipart values live in MultipartForm (PostForm is nil there);
	// merge them so the remap fields reflect the actual form payload.
	if r.MultipartForm != nil {
		for k, vs := range r.MultipartForm.Value {
			if _, dup := raw[k]; dup {
				continue
			}
			put(k, vs)
		}
	}
	fileCount := 0
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, vs := range r.MultipartForm.File {
			fileCount += len(vs)
		}
	}
	first := func(key string) string {
		if r.PostForm != nil {
			if vs, ok := r.PostForm[key]; ok && len(vs) > 0 {
				return vs[0]
			}
			return ""
		}
		if r.MultipartForm != nil {
			if vs, ok := r.MultipartForm.Value[key]; ok && len(vs) > 0 {
				return vs[0]
			}
		}
		return ""
	}
	out := &notifyRequest{FileCount: fileCount}
	if fileCount > 0 {
		out.HasAttach = true
	}
	urls := first("urls")
	if len(urls) > urlsMaxLen {
		urls = ""
	}
	if urls != "" {
		out.URLs = urls
	}
	out.Body = first("body")
	out.Title = first("title")
	out.NotifyType = first("type")
	out.Format = first("format")
	if v := first("tag"); v != "" {
		out.Tag = v
	}
	if v := first("tags"); v != "" {
		out.Tags = v
	}
	// Attach aliases: FORM keys win; the first present alias (attach >
	// attachment > attachments) collects its non-blank values into
	// AttachRaw.
	for _, alias := range []string{"attach", "attachment", "attachments"} {
		var vals []string
		if r.PostForm != nil {
			vals = r.PostForm[alias]
		} else if r.MultipartForm != nil {
			vals = r.MultipartForm.Value[alias]
		}
		if vals == nil {
			continue
		}
		var raws []any
		for _, v := range vals {
			raws = append(raws, v)
			if strings.TrimSpace(v) != "" {
				out.Attach = append(out.Attach, v)
			}
		}
		out.AttachRaw = raws
		out.HasAttach = true
		break
	}
	// Multipart file parts stream into staging, field-sorted with per-field
	// arrival order preserved. Any field name is accepted, mirroring
	// Python's request.FILES handling.
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		var fields []string
		for field := range r.MultipartForm.File {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			for _, fh := range r.MultipartForm.File[field] {
				out.Files = append(out.Files, attach.FilePart(field, fh))
			}
		}
	}
	if len(raw) == 0 && fileCount == 0 && out.Body == "" && out.Title == "" &&
		out.NotifyType == "" && out.Format == "" && out.Tag == nil && out.Tags == nil &&
		len(out.Attach) == 0 && out.URLs == nil {
		return nil, false, raw, nil
	}
	return out, false, raw, nil
}

// parseRemapRules collects ordered '?:src=dst' query rules for the remap
// engine. A rule with an empty source fails the mapping (→ 400).
func parseRemapRules(r *http.Request) ([]remap.Rule, error) {
	return remap.FromRawQuery(r.URL.RawQuery)
}

// rawFieldsForRemap returns the field map handed to the remap engine: the
// raw JSON doc for JSON payloads, the raw form map otherwise.
func rawFieldsForRemap(payload *notifyRequest, rawFields map[string]any, isJSON bool) map[string]any {
	if isJSON {
		if rawFields != nil {
			return rawFields
		}
		return map[string]any{}
	}
	if rawFields != nil {
		return rawFields
	}
	_ = payload
	return map[string]any{}
}

// syncRemappedFields writes remapped values back into the decoded request so
// Apply mutations are observable downstream (validation, send). Keys absent
// from fields were deleted by rules (or never present) and clear the
// corresponding struct field. Attachment aliases resolve with the same
// attach > attachment > attachments priority as decoding; uploaded file parts
// (FileCount) still count as attachments.
func syncRemappedFields(payload *notifyRequest, fields map[string]any, isJSON bool) {
	if payload == nil {
		return
	}
	if v, ok := fields["urls"]; ok {
		if s, isStr := v.(string); !isJSON && isStr && len(s) > urlsMaxLen {
			payload.URLs = nil
		} else {
			payload.URLs = v
		}
	} else {
		payload.URLs = nil
	}
	if v, ok := fields["body"]; ok {
		payload.Body = remappedString(v)
	} else {
		payload.Body = ""
	}
	if v, ok := fields["title"]; ok {
		payload.Title = remappedString(v)
	} else {
		payload.Title = ""
	}
	if v, ok := fields["type"]; ok {
		payload.NotifyType = remappedString(v)
	} else {
		payload.NotifyType = ""
	}
	if v, ok := fields["format"]; ok {
		payload.Format = remappedString(v)
	} else {
		payload.Format = ""
	}
	if v, ok := fields["tag"]; ok {
		payload.Tag = v
	} else {
		payload.Tag = nil
	}
	if v, ok := fields["tags"]; ok {
		payload.Tags = v
	} else {
		payload.Tags = nil
	}
	payload.Attach = nil
	payload.AttachRaw = nil
	payload.HasAttach = payload.FileCount > 0
	for _, alias := range []string{"attach", "attachment", "attachments"} {
		v, ok := fields[alias]
		if !ok || v == nil {
			continue
		}
		payload.Attach = append(payload.Attach, attachStrings(v)...)
		payload.AttachRaw = v
		payload.HasAttach = true
		break
	}
}

// stageAttachments stages the winning attach alias value plus multipart
// file parts under the per-file APPRISE_ATTACH_SIZE cap with the SSRF
// policy applied. The caller removes staged files via attach.CleanupAll.
func (s *Server) stageAttachments(payload *notifyRequest) ([]attach.Staged, error) {
	stager := attach.NewStager(attach.Limits{
		Dir:       s.cfg.AttachDir,
		SizeMB:    s.cfg.AttachSizeMB,
		MaxCount:  s.cfg.MaxAttachments,
		AllowURL:  s.cfg.AttachAllowURLOrDefault(),
		RejectURL: s.cfg.AttachRejectURLOrDefault(),
	})
	return stager.StageRequest(payload.AttachRaw, payload.Files)
}

// remappedString coerces a remapped field value to a scalar string, mirroring
// form decoding (first value wins for multi-value fields).
func remappedString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []string:
		if len(t) > 0 {
			return t[0]
		}
		return ""
	case []any:
		if len(t) == 0 {
			return ""
		}
		if s, ok := t[0].(string); ok {
			return s
		}
		return fmt.Sprintf("%v", t[0])
	default:
		return fmt.Sprintf("%v", v)
	}
}
