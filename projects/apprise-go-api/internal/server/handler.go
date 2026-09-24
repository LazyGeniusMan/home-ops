// Stateless POST /notify handler: Go port of Python apprise-api
// StatelessNotifyView (api/views.py:1724).
//
// serveNotify orchestrates the request: decode (sender.go) → validate
// (validation.go) → send via notify.Sender → negotiated response
// (JSON details | HTML logs | plain text) → outbound webhook.
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
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
		if errors.Is(err, errPayloadTooLarge) {
			s.log.Warn("notify: payload too large", "remote", remoteAddr(r))
			fail(http.StatusRequestHeaderFieldsTooLarge, "JSON Payload provided is to large")
			return
		}
		s.log.Warn("notify: invalid json payload", "remote", remoteAddr(r), "err", err)
		fail(http.StatusBadRequest, "Invalid JSON Payload provided")
		return
	}

	// Apply ':' remap rules from query keys with a ':' prefix (?:src=dst);
	// they remap the decoded payload map before validation.
	rules, err := parseRemapRules(r)
	if err != nil {
		s.log.Warn("notify: remap rules invalid", "remote", remoteAddr(r), "err", redactCredentials(err.Error()))
		fail(http.StatusBadRequest, "Payload field mapping failed")
		return
	}
	if len(rules) > 0 {
		fields := rawFieldsForRemap(payload, rawFields, isJSON)
		// Any Apply error means the mapping failed
		// (→ HTTP 400 "Payload field mapping failed"). Wrap in the
		// errRemapFailed sentinel (%w) so statusCodeOf stays authoritative.
		if err := remap.Apply(fields, rules, s.cfg.WebhookMappingMaxDepth); err != nil {
			mapped := fmt.Errorf("%w: %v", errRemapFailed, err)
			s.log.Warn("notify: remap apply failed", "remote", remoteAddr(r), "err", redactCredentials(mapped.Error()))
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
				// Wrap in errInvalidTag (%w) so statusCodeOf stays
				// authoritative; the user-facing body stays the fixed
				// Python-parity literal below.
				tagErr := fmt.Errorf("%w: %v", errInvalidTag, err)
				s.log.Warn("notify: invalid tag", "remote", remoteAddr(r), "err", redactCredentials(tagErr.Error()))
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
				s.log.Warn("notify: invalid tag type", "remote", remoteAddr(r))
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

	// Stage the winning attach alias value plus multipart file parts to
	// request-scoped temp files; any staging failure is a 400 "Bad
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
			// Log once (redacted: no paths/userinfo); the caller sees
			// only the fixed "Bad Attachment" literal.
			s.log.Warn("notify: bad attachment", "remote", remoteAddr(r), "err", redactCredentials(err.Error()))
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
		// Wrap in errInvalidFormat (%w) so statusCodeOf stays authoritative.
		formatErr := fmt.Errorf("%w: %q", errInvalidFormat, format)
		s.log.Warn("notify: invalid format", "remote", remoteAddr(r), "err", redactCredentials(formatErr.Error()))
		fail(http.StatusBadRequest, "An invalid body input format was specified")
		return
	}

	// Recursion header (views.py:2089): missing → 0; negative or
	// unparseable → 400; over APPRISE_RECURSION_MAX → 406 (not 405).
	recursion := 0
	if raw := strings.TrimSpace(r.Header.Get("X-Apprise-Recursion-Count")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			// Wrap in errInvalidRecursion (%w) so statusCodeOf stays
			// authoritative.
			recErr := fmt.Errorf("%w: %q", errInvalidRecursion, raw)
			s.log.Warn("notify: invalid recursion", "remote", remoteAddr(r), "err", recErr.Error())
			fail(http.StatusBadRequest, "An invalid recursion value was specified")
			return
		}
		if v > s.cfg.RecursionMax {
			// Wrap in errRecursionLimit (%w); the 406 quirk is preserved.
			limitErr := fmt.Errorf("%w: got %d", errRecursionLimit, v)
			s.log.Warn("notify: recursion limit", "remote", remoteAddr(r), "err", limitErr.Error())
			fail(http.StatusNotAcceptable, "The recursion limit has been reached")
			return
		}
		recursion = v
	}

	// X-Apprise-ID is accepted and logged only; the delivery engine carries
	// no per-send identity.
	if uid := strings.TrimSpace(r.Header.Get("X-Apprise-ID")); uid != "" {
		s.log.Debug("notify: request id", "uid", uid)
	}

	// X-Apprise-Log-Level is validated against the allowlist; unknown values
	// fall back to the service default. The level only gates debug output here.
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
		if errors.Is(sendErr, notify.ErrNoTargets) || isNoTargets(sendErr) {
			fail(http.StatusNoContent, "There was no valid URLs provided to notify")
			return
		}
		if attach.IsUnsupportedAttachments(sendErr) && len(urls) > 0 {
			// Wrap with attachment context (%w chain preserved); the
			// user-facing body stays the fixed literal below.
			sendErr = attach.WrapSendError(urls[0], strings.Join(stagedNames, ", "), sendErr)
		}
		// Log once (redacted: strips user:pass@ userinfo); the 424 body
		// carries only the fixed message, never the raw chain.
		s.log.Warn("notify: delivery failed", "remote", remoteAddr(r), "err", redactCredentials(sendErr.Error()))
		respondNotify(w, r, http.StatusFailedDependency, "One or more notifications could not be sent", sendErr, true)
		// Webhook output is sanitized inside fireWebhook (redactCredentials).
		fireWebhook(s, r, false, sendErr)
		return
	}

	// Success: 200 with negotiated logs.
	s.log.Info("notify: delivered", "remote", remoteAddr(r), "targets", len(urls))
	respondNotify(w, r, http.StatusOK, "", nil, false)
	fireWebhook(s, r, true, nil)
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
// mirroring Python's LogCapture JSON shape. Failure details carry only the
// fixed errMsg (never the raw sendErr chain), so user-facing strings carry
// no traces, tokens, or paths.
func respondNotify(w http.ResponseWriter, r *http.Request, status int, errMsg string, _ error, failed bool) {
	accept := r.Header.Get("Accept")
	if accept == "" {
		accept = r.Header.Get("Content-Type")
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	var entry [3]string
	if failed {
		entry = [3]string{"WARNING", now, errMsg}
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

// fireWebhook is the outbound result-hook: it POSTs
// {"source","status":0|1,"output"} to APPRISE_WEBHOOK_URL before the response
// in the caller, logging transport errors only. Non-http(s) URLs are skipped
// with a warning. The output is redacted (no URL userinfo/secrets); X-Apprise
// log/wrap fields reuse the shared validation/sender helpers.
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
		// Send errors are pre-redacted at construction (attach Denied /
		// FetchFailed store userinfo-stripped URLs); redact once more on
		// the joined chain so the outbound hook never carries secrets.
		output = redactCredentials(sendErr.Error())
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

// isNoTargets reports whether err is the zero-survivors condition (kept for
// the message-suffix match; serveNotify also checks errors.Is against
// notify.ErrNoTargets first).
func isNoTargets(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no valid URLs provided to notify")
}
