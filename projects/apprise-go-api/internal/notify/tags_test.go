package notify

import (
	"testing"
)

func TestParseTagExpression(t *testing.T) {
	groups, err := ParseTagExpression("family:2, 3:friends:4")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	if len(groups) != 2 || len(groups[0]) != 1 || len(groups[1]) != 1 {
		t.Fatalf("ParseTagExpression() = %v, want 2 single-token groups", groups)
	}
	if groups[0][0] != "family:2" || groups[1][0] != "3:friends:4" {
		t.Errorf("ParseTagExpression() = %v, want tokens preserved", groups)
	}
}

func TestParseTagExpressionAND(t *testing.T) {
	groups, err := ParseTagExpression("tagA tagC")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("ParseTagExpression() = %v, want 1 two-token group", groups)
	}
	for _, expr := range []string{"a&b", "a+b", "a|b", "a,b"} {
		g, err := ParseTagExpression(expr)
		if err != nil {
			t.Errorf("ParseTagExpression(%q) = %v, want nil", expr, err)
		}
		if len(g) == 0 {
			t.Errorf("ParseTagExpression(%q) = empty, want groups", expr)
		}
	}
}

func TestParseTagExpressionEmpty(t *testing.T) {
	for _, expr := range []string{"", "   "} {
		if got, err := ParseTagExpression(expr); err != nil || got != nil {
			t.Errorf("ParseTagExpression(%q) = %v, %v; want nil, nil", expr, got, err)
		}
	}
}

func TestParseTagExpressionInvalid(t *testing.T) {
	for _, expr := range []string{"family:", "tag!", "a:b:c:d", "semi;colon"} {
		if _, err := ParseTagExpression(expr); err == nil {
			t.Errorf("ParseTagExpression(%q) = nil, want error", expr)
		}
	}
	if _, ok := mustParseErr("family:"); !ok {
		t.Error("ParseTagExpression(family:) error is not *TagError")
	}
}

func mustParseErr(expr string) (error, bool) {
	_, err := ParseTagExpression(expr)
	_, ok := err.(*TagError)
	return err, ok
}

func TestMatchTagsNilMatchesAll(t *testing.T) {
	if !MatchTags(nil, map[string]struct{}{}) {
		t.Error("MatchTags(nil, empty) = false, want true")
	}
	if !MatchTags(nil, map[string]struct{}{"x": {}}) {
		t.Error("MatchTags(nil, tagged) = false, want true")
	}
}

func TestMatchTagsAllShortcut(t *testing.T) {
	all, err := ParseTagExpression("all")
	if err != nil {
		t.Fatalf("ParseTagExpression(all) = %v", err)
	}
	if !MatchTags(all, map[string]struct{}{}) {
		t.Error("MatchTags(all, empty) = false, want true")
	}
	if !MatchTags(all, map[string]struct{}{"mygroup": {}}) {
		t.Error("MatchTags(all, tagged) = false, want true")
	}
}

func TestMatchTagsSpecific(t *testing.T) {
	groups, err := ParseTagExpression("mygroup")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	if MatchTags(groups, map[string]struct{}{}) {
		t.Error("MatchTags(mygroup, empty) = true, want false")
	}
	if !MatchTags(groups, map[string]struct{}{"mygroup": {}}) {
		t.Error("MatchTags(mygroup, mygroup) = false, want true")
	}
}

func TestMatchTagsANDGroup(t *testing.T) {
	groups, err := ParseTagExpression("a b")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	if MatchTags(groups, map[string]struct{}{"a": {}}) {
		t.Error("MatchTags(a b, {a}) = true, want false (AND)")
	}
	if !MatchTags(groups, map[string]struct{}{"a": {}, "b": {}}) {
		t.Error("MatchTags(a b, {a,b}) = false, want true")
	}
}

func TestMatchTagsORGroups(t *testing.T) {
	groups, err := ParseTagExpression("a,b")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	if !MatchTags(groups, map[string]struct{}{"b": {}}) {
		t.Error("MatchTags(a,b, {b}) = false, want true (OR)")
	}
	if MatchTags(groups, map[string]struct{}{"c": {}}) {
		t.Error("MatchTags(a,b, {c}) = true, want false")
	}
}

func TestMatchTagsPriority(t *testing.T) {
	groups, err := ParseTagExpression("3:friends")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	// Plain-string server entries fall back to name-only matching
	// (verified against upstream _token_matches_data).
	if !MatchTags(groups, map[string]struct{}{"friends": {}}) {
		t.Error("MatchTags(3:friends, plain) = false, want true (name fallback)")
	}
	if MatchTags(groups, map[string]struct{}{"4:friends": {}}) {
		t.Error("MatchTags(3:friends, 4:friends) = true, want false (priority-exact)")
	}
	if !MatchTags(groups, map[string]struct{}{"3:friends": {}}) {
		t.Error("MatchTags(3:friends, 3:friends) = false, want true")
	}
	// Name-only token matches a priority-carrying server tag by name.
	plain, err := ParseTagExpression("friends")
	if err != nil {
		t.Fatalf("ParseTagExpression() = %v", err)
	}
	if !MatchTags(plain, map[string]struct{}{"3:friends": {}}) {
		t.Error("MatchTags(friends, 3:friends) = false, want true (name-only)")
	}
}

func TestURLTags(t *testing.T) {
	tags := urlTags("json://localhost/?tag=mygroup,other")
	if _, ok := tags["mygroup"]; !ok {
		t.Errorf("urlTags() = %v, want mygroup", tags)
	}
	if _, ok := tags["other"]; !ok {
		t.Errorf("urlTags() = %v, want other", tags)
	}
	if len(urlTags("json://localhost/")) != 0 {
		t.Error("urlTags(plain) non-empty, want empty")
	}
}

func TestTagMatchesURL(t *testing.T) {
	if !tagMatchesURL("json://localhost/", nil) {
		t.Error("tagMatchesURL(nil filter) = false, want true")
	}
	filter, _ := ParseTagExpression("mygroup")
	if tagMatchesURL("json://localhost/", filter) {
		t.Error("tagMatchesURL(untagged, mygroup) = true, want false")
	}
	if !tagMatchesURL("json://localhost/?tag=mygroup", filter) {
		t.Error("tagMatchesURL(?tag=mygroup, mygroup) = false, want true")
	}
}
