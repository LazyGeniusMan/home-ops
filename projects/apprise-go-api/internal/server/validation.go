// Field validation and coercion for POST /notify payloads.
package server

import (
	"net/http"
	"strings"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

// urlsMaxLen caps the form-path urls field (JSON path bypasses it).
const urlsMaxLen = 1024

// logLevels is the X-Apprise-Log-Level allowlist.
var logLevels = map[string]struct{}{
	"CRITICAL": {}, "ERROR": {}, "WARNING": {}, "INFO": {}, "DEBUG": {}, "TRACE": {},
}

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
// raw strings for the attachment presence check.
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
		// Dict form ({base64,filename}/{url,filename}); presence only.
		return []string{"dict"}
	}
	return nil
}

// listTagFilter converts list-form tags to OR groups, validating each
// token against the Python tag grammar (TAG_TOKEN_RE via
// notify.ValidateTagToken). Python's list path (views.py
// parse_tag_expression skip) passes tokens through unvalidated, so an
// invalid token would silently never match; validating here turns typos
// into a 400 naming the token instead of a silent 204. Empty/blank entries
// are dropped; an all-blank list yields nil (no filter).
func listTagFilter(tags []string) ([]notify.TagGroup, error) {
	groups := make([]notify.TagGroup, 0, len(tags))
	for _, t := range tags {
		if strings.TrimSpace(t) == "" {
			continue
		}
		if err := notify.ValidateTagToken(t); err != nil {
			return nil, err
		}
		groups = append(groups, notify.TagGroup{t})
	}
	if len(groups) == 0 {
		return nil, nil
	}
	return groups, nil
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

// validatedLogLevel applies the X-Apprise-Log-Level allowlist: the header is
// accepted case-insensitively and unknown values fall back to the service
// default.
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

// uploadMaxBytes converts the APPRISE_UPLOAD_MAX_MEMORY_SIZE MiB budget
// to bytes (negative values use their magnitude; zero keeps the 3 MiB
// floor the notify harness and decode paths assume).
func uploadMaxBytes(mb int64) int64 {
	if mb == 0 {
		return 3 << 20
	}
	if mb < 0 {
		mb = -mb
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
