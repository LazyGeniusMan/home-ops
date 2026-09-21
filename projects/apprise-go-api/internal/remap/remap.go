// Package remap implements the ':' third-party webhook payload mapper.
//
// Inbound remap rules arrive as '?:src=dst' query parameters on stateless
// POST /notify. G4 implements the full engine (rename/delete/constant
// assignment, dot-walk plus [N] indexing, depth cap); this file holds the
// rule shape so later features can build on it.
package remap

import (
	"fmt"
	"strings"
)

// Mappable targets: form fields only, plus stateless urls.
var mappableTargets = map[string]struct{}{
	"format": {}, "type": {}, "title": {}, "body": {},
	"attachment": {}, "tag": {}, "tags": {}, "urls": {},
}

// Rule is a single parsed ':src=dst' mapping.
type Rule struct {
	// Source is the payload lookup path (dot-walk with [N] indexes, G4).
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

// IsMappableTarget reports whether name is a legal remap destination.
func IsMappableTarget(name string) bool {
	_, ok := mappableTargets[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// Apply applies rules to fields. Stub for G4: validates rule targets and
// returns nil without mutating fields.
func Apply(fields map[string]any, rules []Rule, maxDepth int) error {
	if maxDepth <= 0 {
		return fmt.Errorf("remap: max depth must be positive, got %d", maxDepth)
	}
	for _, r := range rules {
		if r.Source == "" {
			return fmt.Errorf("remap: rule with empty source")
		}
		if r.Target != "" && !r.IsConstant && !IsMappableTarget(r.Target) {
			return fmt.Errorf("remap: unmappable target %q", r.Target)
		}
	}
	return nil
}
