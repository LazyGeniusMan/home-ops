// Stateless POST /notify handler: Go port of Python apprise-api
// StatelessNotifyView (api/views.py:1724), G2 scope.
//
// Flow: detect JSON payload via content-type regex → parse one of the three
// content types → apply ':' remap rules (stub call; full engine G4) →
// stateless URL fallback → tag/format/type/title query fallbacks → attach
// alias staging to request-scoped temp files (G3) → minimum
// requirements → format/type/recursion validation → allow/deny + send via
// notify.Sender → webhook callback (stub callable; full G4) → negotiated
// response (JSON details | HTML logs | plain text).
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/attach"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/remap"
)

var (
	// mimeIsJSON mirrors Python MIME_IS_JSON (api/utils.py:55).
	mimeIsJSON = regexp.MustCompile(`(?i)(text|application)/(x-)?json`)
	// acceptAll mirrors Python ACCEPT_ALL (api/utils.py:59).
	acceptAll = regexp.MustCompile(`(?i)^\s*([*]/[*]|)\s*$`)
	// htmlAccept mirrors the success content-type selection
	// (views.py:2190): text/* or text/html in Accept → HTML logs.
	htmlAccept = regexp.MustCompile(`(?i)text/(\*|html)`)
)

// urlsMaxLen mirrors Python URLS_MAX_LEN (api/forms.py:56): the form-path
// urls field is capped at 1024 chars; the JSON path bypasses it.
const urlsMaxLen = 1024

// logLevels mirrors the X-Apprise-Log-Level allowlist (views.py:2212).
var logLevels = map[string]struct{}{
	"CRITICAL": {}, "ERROR": {}, "WARNING": {}, "INFO": {}, "DEBUG": {}, "TRACE": {},
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
	// (string, list, or dict) handed to the G3 stager.
	AttachRaw any
	// Files holds multipart file parts in arrival order for the stager.
	Files []attach.Incoming
	// HasAttach reports any attach/attachment/attachments alias or file part.
	HasAttach bool
	// FileCount counts uploaded file parts (G3 stages them).
	FileCount int
}

// serveNotify serves POST /notify and POST /notify/ (POST-only; GET→405).
func (s *Server) serveNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	jsonResponse := isJSONResponse(r)
	fail := func(status int, msg string) {
		if jsonResponse {
			writeJSON(w, status, map[string]any{"error": msg})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, msg)
	}

	// Parse the payload by content type.
	payload, isJSON, rawFields, err := s.decodePayload(r)
	if err != nil {
		if err == errPayloadTooLarge {
			fail(http.StatusRequestHeaderFieldsTooLarge, "JSON Payload provided is to large")
			return
		}
		fail(http.StatusBadRequest, "Invalid JSON Payload provided")
		return
	}

	// Apply ':' remap rules (G4 engine). Rules come from query keys with
	// a ':' prefix (?:src=dst); they remap the decoded payload map before
	// validation, and mapped values flow back into the request struct below.
	rules, err := parseRemapRules(r)
	if err != nil {
		fail(http.StatusBadRequest, "Payload field mapping failed")
		return
	}
	if len(rules) > 0 {
		fields := rawFieldsForRemap(payload, rawFields, isJSON)
		// Any Apply error means the mapping failed
		// (→ HTTP 400 "Payload field mapping failed").
		if err := remap.Apply(fields, rules, s.cfg.WebhookMappingMaxDepth); err != nil {
			fail(http.StatusBadRequest, "Payload field mapping failed")
			return
		}
		syncRemappedFields(payload, fields, isJSON)
	}

	if payload == nil {
		fail(http.StatusBadRequest, "Bad FORM Payload provided")
		return
	}

	// Stateless URL fallback (views.py:1869).
	urlsRaw := payload.URLs
	if isEmptyURLs(urlsRaw) && s.cfg.StatelessURLs != "" {
		urlsRaw = s.cfg.StatelessURLs
	}
	urls := splitURLs(urlsRaw)

	// Query fallbacks for tag/tags/format/type/title (body wins).
	tagRaw := firstNonEmpty(payload.Tag, payload.Tags)
	if isEmptyValue(tagRaw) {
		if q := r.URL.Query().Get("tag"); q != "" {
			tagRaw = q
		} else if q := r.URL.Query().Get("tags"); q != "" {
			tagRaw = q
		}
	}
	format := payload.Format
	if format == "" {
		format = r.URL.Query().Get("format")
	}
	notifyType := payload.NotifyType
	if notifyType == "" {
		notifyType = r.URL.Query().Get("type")
	}
	title := payload.Title
	if title == "" {
		title = r.URL.Query().Get("title")
	}

	// Tag grammar validation. Lists pass straight through (Python: list
	// payloads skip parse_tag_expression); strings are parsed to OR/AND
	// structure; anything else is a 400.
	var tagFilter []notify.TagGroup
	switch tag := tagRaw.(type) {
	case nil:
		// No filter.
	case string:
		if tag != "" {
			groups, err := notify.ParseTagExpression(tag)
			if err != nil {
				s.log.Warn("notify: invalid tag", "remote", remoteAddr(r))
				fail(http.StatusBadRequest, "Unsupported characters found in tag definition")
				return
			}
			tagFilter = groups
		}
	case []string:
		tagFilter = listTagFilter(tag)
	case []any:
		strs := make([]string, 0, len(tag))
		for _, v := range tag {
			sv, ok := v.(string)
			if !ok {
				fail(http.StatusBadRequest, "Unsupported characters found in tag definition")
				return
			}
			strs = append(strs, sv)
		}
		tagFilter = listTagFilter(strs)
	default:
		s.log.Warn("notify: invalid tag type", "remote", remoteAddr(r))
		fail(http.StatusBadRequest, "Unsupported characters found in tag definition")
		return
	}

	// Attach staging (G3): the winning attach alias value plus any multipart
	// file parts are staged to request-scoped temp files under
	// APPRISE_ATTACH_DIR. Staged paths feed the sender; temp files are
	// removed when the request ends. Any staging failure (malformed entry,
	// SSRF denial, fetch failure, over-limit file) is a 400 "Bad
	// Attachment" via attach.StatusCodeOf.
	var attachPaths []string
	var stagedNames []string
	if payload.HasAttach && len(payload.Attach) == 0 && payload.FileCount == 0 {
		// Alias declared but empty (e.g. attach= with blank value):
		// Python parse_attachments decrements blank entries and yields no
		// attach — body-required rule then applies.
	} else if payload.HasAttach {
		staged, err := s.stageAttachments(payload)
		if err != nil {
			s.log.Warn("notify: bad attachment", "remote", remoteAddr(r), "err", err)
			fail(attach.StatusCodeOf(err), "Bad Attachment")
			return
		}
		defer attach.CleanupAll(staged)
		attachPaths = attach.Paths(staged)
		stagedNames = attach.Names(staged)
		// Staged presence satisfies the body-required rule below.
		payload.HasAttach = attach.HasAttachment(staged, payload.AttachRaw)
	}

	// Minimum requirements: body or attach, and a valid type
	// (views.py:2016). Default type is info; anything outside
	// info|success|warning|failure is a 400.
	body := payload.Body
	ntype := strings.ToLower(strings.TrimSpace(notifyType))
	if ntype == "" {
		ntype = "info"
	}
	if (strings.TrimSpace(body) == "" && !payload.HasAttach) || !validNotifyType(ntype) {
		s.log.Warn("notify: payload lacks minimum requirements", "remote", remoteAddr(r))
		fail(http.StatusBadRequest, "Payload lacks minimum requirements")
		return
	}

	// Body format: empty/missing means "ignore" (passes); otherwise it must
	// be text|markdown|html (views.py:2040).
	bodyFormat := strings.ToLower(strings.TrimSpace(format))
	if bodyFormat == "" {
		bodyFormat = "text"
	} else if !validInputFormat(bodyFormat) {
		s.log.Warn("notify: invalid format", "remote", remoteAddr(r), "format", format)
		fail(http.StatusBadRequest, "An invalid body input format was specified")
		return
	}

	// Recursion header (views.py:2089): missing → 0; negative or
	// unparseable → 400; over APPRISE_RECURSION_MAX → 406 (not 405).
	recursion := 0
	if raw := strings.TrimSpace(r.Header.Get("X-Apprise-Recursion-Count")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			s.log.Warn("notify: invalid recursion", "remote", remoteAddr(r), "value", raw)
			fail(http.StatusBadRequest, "An invalid recursion value was specified")
			return
		}
		if v > s.cfg.RecursionMax {
			s.log.Warn("notify: recursion limit", "remote", remoteAddr(r), "value", v)
			fail(http.StatusNotAcceptable, "The recursion limit has been reached")
			return
		}
		recursion = v
	}

	// X-Apprise-ID is accepted and passed through (uid wiring point: the
	// engine exposes no per-send identity knob, so it is logged only).
	if uid := strings.TrimSpace(r.Header.Get("X-Apprise-ID")); uid != "" {
		s.log.Debug("notify: request id", "uid", uid)
	}

	// X-Apprise-Log-Level is validated against the allowlist; unknown
	// values fall back to the service default (Python resets to the
	// configured apprise level). G2 synthesizes its own log records, so
	// the level only gates debug output here.
	_ = validatedLogLevel(r.Header.Get("X-Apprise-Log-Level"), s.cfg.LogLevel)

	// Send via the engine. Zero surviving targets → 204; any delivery
	// error → 424 with negotiated logs/details. A target that cannot
	// carry attachments fails the send with the staged filename attached
	// so the failure is never silent.
	req := notify.Request{
		URLs:               urls,
		Body:               body,
		Title:              title,
		NotifyType:         ntype,
		InputFormat:        bodyFormat,
		Tag:                tagFilter,
		Attachments:        attachPaths,
		AttachmentMaxBytes: s.cfg.AttachSizeBytes(),
		DenyServices:       s.cfg.DenyServices,
		AllowServices:      s.cfg.AllowServices,
		RecursionCount:     recursion,
	}
	result, sendErr := s.sender.Send(r.Context(), req)
	_ = result
	if sendErr != nil {
		if isNoTargets(sendErr) {
			fail(http.StatusNoContent, "There was no valid URLs provided to notify")
			return
		}
		if attach.IsUnsupportedAttachments(sendErr) && len(urls) > 0 {
			sendErr = attach.WrapSendError(urls[0], strings.Join(stagedNames, ", "), sendErr)
		}
		s.log.Warn("notify: delivery failed", "remote", remoteAddr(r), "err", sendErr)
		respondNotify(w, r, http.StatusFailedDependency, "One or more notifications could not be sent", sendErr, true)
		fireWebhook(s, r, false, sendErr)
		return
	}

	// Success: 200 with negotiated logs.
	s.log.Info("notify: delivered", "remote", remoteAddr(r), "targets", len(urls))
	respondNotify(w, r, http.StatusOK, "", nil, false)
	fireWebhook(s, r, true, nil)
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
	// winner into Attach and keep its decoded shape in AttachRaw for the
	// G3 stager. FORM beats JSON — handled in decodeFormPayload (JSON path
	// has no FORM keys by construction).
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
	// Attach aliases: FORM keys win; first present alias (attach >
	// attachment > attachments) collects its non-blank values and keeps
	// its winning value in AttachRaw for the G3 stager.
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

// stageAttachments stages a request's attachments through the G3 stager:
// the winning attach alias value plus multipart file parts, bounded by the
// per-file APPRISE_ATTACH_SIZE cap with the SSRF allow/reject policy
// applied to remote URLs. Staged temp files live under APPRISE_ATTACH_DIR
// and the caller removes them via attach.CleanupAll.
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

// isJSONResponse mirrors Python is_json_response (api/utils.py:63): Accept:
// application/json forces JSON; missing/wildcard Accept falls back to the
// request Content-Type.
func isJSONResponse(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	ct := r.Header.Get("Content-Type")
	return mimeIsJSON.MatchString(accept) ||
		(acceptAll.MatchString(accept) && mimeIsJSON.MatchString(ct))
}

// respondNotify writes the negotiated success/failure body:
// JSON {"error","details"} | HTML <ul class="logs"> | plain text lines.
// Log records are synthesized server-side as [level, date, message] entries
// mirroring Python's LogCapture JSON shape.
func respondNotify(w http.ResponseWriter, r *http.Request, status int, errMsg string, sendErr error, failed bool) {
	accept := r.Header.Get("Accept")
	if accept == "" {
		accept = r.Header.Get("Content-Type")
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	var entry [3]string
	if failed {
		entry = [3]string{"WARNING", now, errMsg}
		if sendErr != nil {
			entry[2] = fmt.Sprintf("%s: %v", errMsg, sendErr)
		}
	} else {
		entry = [3]string{"INFO", now, "Delivered Stateless Notification(s)"}
	}
	details := [][3]string{entry}
	switch {
	case isJSONResponse(r):
		payload := map[string]any{"error": nil, "details": details}
		if failed {
			payload["error"] = errMsg
		}
		writeJSON(w, status, payload)
	case htmlAccept.MatchString(accept):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `<ul class="logs">`)
		for _, e := range details {
			_, _ = fmt.Fprintf(w, `<li class="log_%s"><div class="log_time">%s</div><div class="log_level">%s</div><div class="log_msg">%s</div></li>`,
				html.EscapeString(e[0]), html.EscapeString(e[1]), html.EscapeString(e[0]), html.EscapeString(e[2]))
		}
		_, _ = io.WriteString(w, `</ul>`)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		for _, e := range details {
			_, _ = fmt.Fprintf(w, "%s [%s] %s: %s\n", e[1], e[0], "notify", e[2])
		}
	}
}

// fireWebhook is the outbound result-hook wiring point (full G4 delivery
// lands separately): it POSTs {"source","status":0|1,"output"} to
// APPRISE_WEBHOOK_URL before the response in the caller, logging transport
// errors only. Non-http(s) URLs are skipped with a warning.
func fireWebhook(s *Server, r *http.Request, ok bool, sendErr error) {
	url := strings.TrimSpace(s.cfg.WebhookURL)
	if url == "" {
		return
	}
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		s.log.Warn("notify: invalid webhook url", "remote", remoteAddr(r))
		return
	}
	status := 0
	if !ok {
		status = 1
	}
	output := ""
	if sendErr != nil {
		output = sendErr.Error()
	}
	body, _ := json.Marshal(map[string]any{
		"source": remoteAddr(r),
		"status": status,
		"output": output,
	})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		s.log.Warn("notify: webhook build failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Apprise-API")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.log.Warn("notify: webhook delivery failed", "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
}

// Helpers.

func isEmptyURLs(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []string:
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				return false
			}
		}
		return true
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []string:
		return len(t) == 0
	case []any:
		return len(t) == 0
	default:
		return false
	}
}

func firstNonEmpty(a, b any) any {
	if !isEmptyValue(a) {
		return a
	}
	return b
}

// splitURLs splits a urls value (string or list) on commas/whitespace,
// mirroring Python's apprise URL splitting. Non-string scalars are dropped.
func splitURLs(v any) []string {
	var out []string
	switch t := v.(type) {
	case string:
		out = strings.FieldsFunc(t, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
	case []string:
		for _, s := range t {
			out = append(out, strings.FieldsFunc(s, func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
			})...)
		}
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, strings.FieldsFunc(s, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
				})...)
			}
		}
	}
	var cleaned []string
	for _, s := range out {
		if s = strings.TrimSpace(s); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	return cleaned
}

// attachStrings flattens a JSON attach value (string, list, or dict) into
// raw strings for the stub presence check.
func attachStrings(v any) []string {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) != "" {
			return []string{t}
		}
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, attachStrings(e)...)
		}
		return out
	case []string:
		var out []string
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case map[string]any:
		// G3 dict form ({base64,filename}/{url,filename}); presence only.
		return []string{"dict"}
	}
	return nil
}

// listTagFilter passes list-form tags straight through (Python: list
// payloads skip parse_tag_expression), one single-token OR group each.
func listTagFilter(tags []string) []notify.TagGroup {
	groups := make([]notify.TagGroup, 0, len(tags))
	for _, t := range tags {
		groups = append(groups, notify.TagGroup{t})
	}
	return groups
}

func validNotifyType(t string) bool {
	switch t {
	case "info", "success", "warning", "failure":
		return true
	default:
		return false
	}
}

func validInputFormat(f string) bool {
	switch f {
	case "text", "markdown", "html":
		return true
	default:
		return false
	}
}

func isNoTargets(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no valid URLs provided to notify")
}

func uploadMaxBytes(mb int64) int64 {
	if mb <= 0 {
		return 3 << 20
	}
	return mb << 20
}

func remoteAddr(r *http.Request) string {
	if r == nil || r.RemoteAddr == "" {
		return "unknown"
	}
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok && host != "" {
		return host
	}
	return r.RemoteAddr
}

// validatedLogLevel keeps the X-Apprise-Log-Level allowlist visible: the
// header is accepted case-insensitively and unknown values fall back to the
// service default (Python resets to the configured apprise level). G2
// synthesizes its own log records, so the level only gates debug output.
func validatedLogLevel(raw, def string) string {
	level := strings.ToUpper(strings.TrimSpace(raw))
	if level == "" {
		level = strings.ToUpper(strings.TrimSpace(def))
	}
	if _, ok := logLevels[level]; ok {
		return level
	}
	if _, ok := logLevels[strings.ToUpper(strings.TrimSpace(def))]; ok {
		return strings.ToUpper(strings.TrimSpace(def))
	}
	return "WARNING"
}
