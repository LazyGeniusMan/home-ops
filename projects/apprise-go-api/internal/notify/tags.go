// Package notify tag routing: parse and match stateless tag filters.
//
// The grammar mirrors Python apprise-api parse_tag_expression
// (views.py:88) and upstream is_exclusive_match (apprise 1.13.1,
// utils/logic.py), verified empirically against the pinned library:
//
//   - ',' and '|' separate OR groups; whitespace, '&', '+' separate AND
//     tokens within a group.
//   - Tokens use [priority:]name[:retry]; validation is
//     ^[a-z0-9][a-z0-9_-]*$ per component (case-insensitive).
//   - A nil filter means "no filter": every target is notified.
//   - The filter token "all" matches every server (match_all shortcut);
//     an empty filter never matches (returns false unless the server is
//     also untagged — which cannot happen here since our servers carry
//     no tags, so empty means no match).
//
// Stateless servers arrive with no configured tags (Python instantiate()
// sets results["tag"] = set(parse_list(tag)) with tag=None for stateless
// URLs), so matching reduces to: filter token "all" (case-insensitive,
// without priority prefix) matches; any other token does not. A URL's own
// ?tag= query parameter counts as server tags (parsed the same way).
package notify

import (
	"regexp"
	"strings"
)

// TagGroup is one OR group: every token in Tokens must match (AND).
type TagGroup []string

var (
	// tagValidationRe mirrors TAG_VALIDATION_RE (views.py:73).
	tagValidationRe = regexp.MustCompile(`(?i)^[a-z0-9\s|, _:+&-]+$`)
	// tagOrDelimRe mirrors TAG_OR_DELIM_RE (views.py:76).
	tagOrDelimRe = regexp.MustCompile(`\s*[|,]\s*`)
	// tagAndDelimRe mirrors TAG_AND_DELIM_RE (views.py:79).
	tagAndDelimRe = regexp.MustCompile(`[\s&+]+`)
	// tagTokenRe mirrors TAG_TOKEN_RE (views.py:82).
	tagTokenRe = regexp.MustCompile(`(?i)^(?:[0-9]+:)?[a-z0-9][a-z0-9_-]*(?::[0-9]+)?$`)
)

// ParseTagExpression converts a user-provided tag expression into OR/AND
// structure. Commas and pipes are OR separators; whitespace, ampersands,
// and plus signs are AND separators. An empty expression yields nil
// (no filter). Invalid characters or tokens return an error.
func ParseTagExpression(tag string) ([]TagGroup, error) {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return nil, nil
	}
	if !tagValidationRe.MatchString(trimmed) {
		return nil, &TagError{Expr: tag}
	}
	var groups []TagGroup
	for _, group := range tagOrDelimRe.Split(trimmed, -1) {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		var tokens []string
		for _, token := range tagAndDelimRe.Split(group, -1) {
			if token == "" {
				continue
			}
			if !tagTokenRe.MatchString(token) {
				return nil, &TagError{Expr: tag}
			}
			tokens = append(tokens, token)
		}
		if len(tokens) == 0 {
			continue
		}
		groups = append(groups, tokens)
	}
	return groups, nil
}

// TagError reports an invalid tag expression.
type TagError struct {
	// Expr is the offending expression (truncated by callers for logs).
	Expr string
}

func (e *TagError) Error() string {
	return "notify: unsupported characters found in tag definition"
}

// MatchTags reports whether parsed filter groups match server tags.
// Nil groups mean no filter (match everything). Otherwise each OR group is
// tried in order: a group matches when every token matches per
// matchTagToken. This mirrors is_exclusive_match with match_always='always'
// folded into the "all" shortcut below.
func MatchTags(groups []TagGroup, serverTags map[string]struct{}) bool {
	if groups == nil {
		return true
	}
	for _, group := range groups {
		if matchTagGroup(group, serverTags) {
			return true
		}
	}
	return false
}

// matchTagGroup reports whether every token in group matches serverTags.
func matchTagGroup(group TagGroup, serverTags map[string]struct{}) bool {
	if len(group) == 0 {
		return len(serverTags) == 0
	}
	for _, token := range group {
		if !matchTagToken(token, serverTags) {
			return false
		}
	}
	return true
}

// matchTagToken mirrors _token_matches_data (verified empirically): the
// token "all" (match_all, case-insensitive, no priority prefix) matches
// every server. A name-only token matches any server tag with the same
// name regardless of stored priority. A priority-prefixed token matches
// AppriseTag server entries only on exact name+priority; plain-string
// server entries fall back to name-only matching.
func matchTagToken(token string, serverTags map[string]struct{}) bool {
	name, priority, hasPriority := splitTagToken(token)
	if !hasPriority && strings.EqualFold(name, "all") {
		return true
	}
	if !hasPriority {
		return matchTagName(name, serverTags)
	}
	for tag := range serverTags {
		if n, p, hp := splitTagToken(tag); hp {
			if strings.EqualFold(n, name) && p == priority {
				return true
			}
		} else if strings.EqualFold(tag, name) {
			return true
		}
	}
	return false
}

// matchTagName reports whether any server tag carries name (bare,
// case-insensitive), ignoring priority/retry decorations.
func matchTagName(name string, serverTags map[string]struct{}) bool {
	for tag := range serverTags {
		if n, _, _ := splitTagToken(tag); strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// splitTagToken splits [priority:]name[:retry] into its parts.
func splitTagToken(token string) (name string, priority int, hasPriority bool) {
	rest := token
	if head, tail, ok := strings.Cut(rest, ":"); ok && isDigits(head) {
		hasPriority = true
		priority = atoiDigits(head)
		rest = tail
	}
	if head, tail, ok := strings.Cut(rest, ":"); ok && isDigits(tail) {
		rest = head
	}
	return rest, priority, hasPriority
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func atoiDigits(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// tagMatchesURL reports whether rawURL survives the stateless tag filter.
// The URL's own ?tag= query values count as server tags; the parsed request
// filter applies on top. A nil filter matches everything.
func tagMatchesURL(rawURL string, filter []TagGroup) bool {
	if filter == nil {
		return true
	}
	return MatchTags(filter, urlTags(rawURL))
}

// urlTags extracts the server-side tags from a notification URL's ?tag=
// query parameter (comma/space separated), lowercased for matching.
func urlTags(rawURL string) map[string]struct{} {
	tags := map[string]struct{}{}
	_, query, _ := strings.Cut(rawURL, "?")
	query, _, _ = strings.Cut(query, "#")
	for _, field := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		key, value, _ := strings.Cut(field, "=")
		if !strings.EqualFold(strings.TrimSpace(key), "tag") {
			continue
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		}) {
			if token = strings.TrimSpace(token); token != "" {
				tags[strings.ToLower(token)] = struct{}{}
			}
		}
	}
	return tags
}
