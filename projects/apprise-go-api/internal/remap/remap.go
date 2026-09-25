// Package remap implements the ':' webhook payload mapper: '?:src=dst'
// params remap the decoded payload before validation (failures → 400).
package remap

import (
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Mappable targets: form fields only, plus stateless urls.
var mappableTargets = map[string]struct{}{
	"format": {}, "type": {}, "title": {}, "body": {},
	"attachment": {}, "tag": {}, "tags": {}, "urls": {},
}

// Rule is a single parsed ':src=dst' mapping.
type Rule struct {
	// Source is the payload lookup path (dot-walk with [N] indexes).
	Source string
	// Target is the destination form field, or empty for a delete rule.
	Target string
	// Constant, when IsConstant, is assigned verbatim instead of looked up.
	Constant string
	// IsConstant marks a ':expected=fixed string' assignment rule.
	IsConstant bool
}

// Parse parses a single mapping value in "src=dst" form (the leading ':'
// query prefix is stripped by the caller).
func Parse(raw string) (Rule, error) {
	src, dst, ok := strings.Cut(raw, "=")
	if !ok {
		return Rule{}, fmt.Errorf("remap: invalid mapping %q: want src=dst", raw)
	}
	src = strings.TrimSpace(src)
	if src == "" {
		return Rule{}, fmt.Errorf("remap: invalid mapping %q: empty source", raw)
	}
	return Rule{Source: src, Target: strings.TrimSpace(dst)}, nil
}

// IsMappableTarget reports whether name is a legal remap destination
// (case-insensitive; Apply resolves targets with exact matching).
func IsMappableTarget(name string) bool {
	_, ok := mappableTargets[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// isExpected reports whether name is an expected form field (exact,
// case-sensitive).
func isExpected(name string) bool {
	_, ok := mappableTargets[name]
	return ok
}

// logger is the package logger.
var logger = slog.Default()

// Step is one traversal step of a parsed source path: either a dict-key
// lookup (IsIndex=false) or an array-index dereference (IsIndex=true).
type Step struct {
	Key     string
	Index   int
	IsIndex bool
}

// FromRawQuery extracts ordered ':' remap rules from a raw query string.
// Only ':' keys participate (missing '=' is a delete rule); rule order
// follows first appearance.
func FromRawQuery(rawQuery string) ([]Rule, error) {
	if rawQuery == "" {
		return nil, nil
	}
	var rules []Rule
	for _, pair := range strings.Split(rawQuery, "&") {
		name, value, hasValue := strings.Cut(pair, "=")
		decodedName, err := url.QueryUnescape(name)
		if err != nil {
			return nil, fmt.Errorf("remap: bad query parameter %q: %w", name, err)
		}
		if !strings.HasPrefix(decodedName, ":") {
			continue
		}
		src := decodedName[1:]
		var dst string
		if hasValue {
			decodedValue, err := url.QueryUnescape(value)
			if err != nil {
				return nil, fmt.Errorf("remap: bad mapping value for %q: %w", decodedName, err)
			}
			dst = decodedValue
		}
		if strings.TrimSpace(src) == "" {
			return nil, fmt.Errorf("remap: invalid mapping %q: empty source", pair)
		}
		rules = append(rules, Rule{Source: src, Target: strings.TrimSpace(dst)})
	}
	return rules, nil
}

// Apply applies rules to fields in place (errors → 400). Path sources
// with empty/non-mappable targets are silent no-ops; flat sources
// delete/rename/swap, else assign verbatim as a constant.
func Apply(fields map[string]any, rules []Rule, maxDepth int) error {
	if maxDepth <= 0 {
		return fmt.Errorf("remap: max depth must be positive, got %d", maxDepth)
	}
	for _, r := range rules {
		if isPathSource(r.Source) {
			steps, err := ParsePath(r.Source)
			if err != nil {
				logger.Warn("remap: bad mapping path",
					slog.String("path", r.Source), slog.String("error", err.Error()))
				return fmt.Errorf("remap: bad mapping path %q: %w", r.Source, err)
			}
			if len(steps) > maxDepth {
				logger.Warn("remap: mapping path exceeds maximum depth",
					slog.String("path", r.Source), slog.Int("max_depth", maxDepth))
				return fmt.Errorf("remap: mapping path %q exceeds maximum depth %d", r.Source, maxDepth)
			}
			value, ok := GetNested(fields, steps, r.Source)
			if !ok {
				return fmt.Errorf("remap: mapping path %q not found in payload", r.Source)
			}
			// Nested-source + empty/non-expected target = silent no-op.
			// Target matching is exact (case-sensitive), mirroring
			// Python's `value in expected_keys`.
			if isExpected(r.Target) {
				fields[r.Target] = value
			}
			continue
		}
		if r.Source == "" {
			return fmt.Errorf("remap: rule with empty source")
		}
		if r.Target == "" {
			delete(fields, r.Source)
			continue
		}
		// Target matching is exact (case-sensitive).
		if isExpected(r.Target) {
			target := r.Target
			srcVal, srcOK := fields[r.Source]
			if !srcOK {
				continue
			}
			if _, targetExists := fields[target]; !targetExists || r.Source == target {
				fields[target] = srcVal
				if r.Source != target {
					delete(fields, r.Source)
				}
				continue
			}
			if !isExpected(r.Source) {
				fields[target] = srcVal
				delete(fields, r.Source)
				continue
			}
			// Both sides are expected fields holding values: swap.
			fields[target], fields[r.Source] = fields[r.Source], fields[target]
			continue
		}
		// Constant assignment: source names an expected field or present key.
		if isExpected(r.Source) || hasKey(fields, r.Source) {
			fields[r.Source] = r.Target
			continue
		}
	}
	return nil
}

// isPathSource reports whether src must be treated as a nested path rather
// than a flat top-level key.
func isPathSource(src string) bool {
	return strings.ContainsAny(src, ".[]")
}

// ParsePath parses a mapping source into ordered traversal steps
// ("event.title" → [key event, key title], "items[0]" → [key items, index 0]).
func ParsePath(key string) ([]Step, error) {
	var steps []Step
	for _, segment := range strings.Split(key, ".") {
		if segment == "" {
			return nil, fmt.Errorf("empty segment in path %q", key)
		}
		if !strings.ContainsAny(segment, "[]") {
			steps = append(steps, Step{Key: segment})
			continue
		}
		open := strings.Index(segment, "[")
		if open == -1 {
			return nil, fmt.Errorf("malformed bracket notation at %q (unexpected ']' with no matching '[')", segment)
		}
		name, rest := segment[:open], segment[open:]
		if strings.Contains(name, "]") {
			return nil, fmt.Errorf("malformed bracket notation at %q; unexpected ']' before '['", segment)
		}
		if name != "" {
			steps = append(steps, Step{Key: name})
		}
		for len(rest) > 0 {
			if !strings.HasPrefix(rest, "[") {
				return nil, fmt.Errorf("malformed bracket notation at %q; expected [N] where N is a non-negative integer", segment)
			}
			close := strings.Index(rest, "]")
			if close == -1 {
				return nil, fmt.Errorf("malformed bracket notation at %q; expected [N] where N is a non-negative integer", segment)
			}
			rawIdx := rest[1:close]
			idx, err := strconv.Atoi(rawIdx)
			if err != nil || idx < 0 || !isDigits(rawIdx) {
				// Mirrors Python's \[(\d+)\]: one or more ASCII digits.
				// Rejects "", "abc", "-1", "+1", " 1", "1.5".
				return nil, fmt.Errorf("malformed bracket notation at %q; expected [N] where N is a non-negative integer", segment)
			}
			steps = append(steps, Step{Index: idx, IsIndex: true})
			rest = rest[close+1:]
		}
	}
	return steps, nil
}

// GetNested walks payload along steps; missing keys, non-indexable nodes,
// or out-of-range indexes log a WARNING and return false.
func GetNested(payload any, steps []Step, source string) (any, bool) {
	current := payload
	for _, s := range steps {
		if !s.IsIndex {
			m, ok := current.(map[string]any)
			if !ok {
				logger.Warn("remap: mapping path not found in payload (not a mapping)",
					slog.String("path", source), slog.String("key", s.Key))
				return nil, false
			}
			next, ok := m[s.Key]
			if !ok {
				logger.Warn("remap: mapping path not found in payload",
					slog.String("path", source), slog.String("key", s.Key))
				return nil, false
			}
			current = next
			continue
		}
		v := reflect.ValueOf(current)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			got := "<nil>"
			if current != nil {
				got = reflect.TypeOf(current).String()
			}
			logger.Warn("remap: mapping path index is not indexable",
				slog.String("path", source), slog.Int("index", s.Index), slog.String("got", got))
			return nil, false
		}
		if s.Index >= v.Len() {
			logger.Warn("remap: mapping path index out of range",
				slog.String("path", source), slog.Int("index", s.Index), slog.Int("length", v.Len()))
			return nil, false
		}
		current = v.Index(s.Index).Interface()
	}
	return current, true
}

// hasKey reports whether fields holds key (exact, case-sensitive).
func hasKey(fields map[string]any, key string) bool {
	_, ok := fields[key]
	return ok
}

// isDigits reports whether s is one or more ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
