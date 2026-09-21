package remap

import (
	"maps"
	"reflect"
	"testing"
)

// mustParse is a test helper: Parse never fails for src=dst-shaped input.
func mustParse(t *testing.T, raw string) Rule {
	t.Helper()
	r, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", raw, err)
	}
	return r
}

func applyRules(t *testing.T, payload map[string]any, raws []string, maxDepth int) error {
	t.Helper()
	rules := make([]Rule, 0, len(raws))
	for _, raw := range raws {
		rules = append(rules, mustParse(t, raw))
	}
	return Apply(payload, rules, maxDepth)
}

func TestApplyFlatRemap(t *testing.T) {
	// Mirrors test_remap_fields test 1: renames + delete + constant + skip.
	payload := map[string]any{
		"as": "markdown", "subject": "title", "content": "# body",
		"tag": "", "unknown": "hmm", "attachment": "", "garbage": "",
	}
	err := applyRules(t, payload, []string{
		"as=format", "subject=title", "content=body",
		"unknown=missing", "attachment=", "garbage=", "tag=test",
	}, 5)
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := map[string]any{
		"tag": "test", "unknown": "missing", "format": "markdown",
		"title": "title", "body": "# body",
	}
	if !reflect.DeepEqual(payload, want) {
		t.Errorf("Apply() = %v, want %v", payload, want)
	}
}

func TestApplyDoubleMapToBody(t *testing.T) {
	// Mirrors test_remap_fields test 2: content->body, message->body
	// (target occupied), body constant.
	payload := map[string]any{
		"as": "markdown", "subject": "title", "content": "# content body",
		"message": "# message body", "body": "another set of data",
	}
	err := applyRules(t, payload, []string{
		"content=body", "message=body", "body=another set of data",
	}, 5)
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := map[string]any{
		"as": "markdown", "subject": "title", "body": "another set of data",
	}
	if !reflect.DeepEqual(payload, want) {
		t.Errorf("Apply() = %v, want %v", payload, want)
	}
}

func TestApplySwap(t *testing.T) {
	// Mirrors test_remap_fields tests 3-5: both-exist expected fields swap.
	payload := map[string]any{"format": "markdown", "title": "body", "body": "# title"}
	if err := applyRules(t, payload, []string{"title=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := map[string]any{"format": "markdown", "title": "# title", "body": "body"}
	if !reflect.DeepEqual(payload, want) {
		t.Errorf("Apply() = %v, want %v", payload, want)
	}

	// Missing target: rename replaces.
	payload = map[string]any{"format": "markdown", "title": "body"}
	if err := applyRules(t, payload, []string{"title=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want = map[string]any{"format": "markdown", "body": "body"}
	if !reflect.DeepEqual(payload, want) {
		t.Errorf("Apply() = %v, want %v", payload, want)
	}

	// Non-expected source wins over the existing target value.
	payload = map[string]any{"format": "markdown", "content": "the message", "body": "to-be-replaced"}
	if err := applyRules(t, payload, []string{"content=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want = map[string]any{"format": "markdown", "body": "the message"}
	if !reflect.DeepEqual(payload, want) {
		t.Errorf("Apply() = %v, want %v", payload, want)
	}
}

func TestApplyNoAlignmentNoop(t *testing.T) {
	// Mirrors test_remap_fields test 6: nothing aligns, payload untouched.
	payload := map[string]any{
		"format": "markdown", "type": "info", "title": "",
		"body": "## test notifiction", "attachment": nil,
		"tag": "general", "tags": "",
	}
	orig := maps.Clone(payload)
	err := applyRules(t, payload, []string{"payload=body", "fmt=format", "extra=tag"}, 5)
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if !reflect.DeepEqual(payload, orig) {
		t.Errorf("Apply() = %v, want unchanged %v", payload, orig)
	}
}

func TestApplyQueryRuleExamples(t *testing.T) {
	// Brief acceptance examples: form-style and JSON-style flat rules.
	payload := map[string]any{"subject": "hi", "payload": "hello", "urls": "json://localhost"}
	if err := applyRules(t, payload, []string{"subject=title", "payload=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["title"] != "hi" || payload["body"] != "hello" {
		t.Errorf("Apply() = %v, want title/body remapped", payload)
	}

	payload = map[string]any{
		"subject": "hi", "payload": "hello", "href": "json://localhost",
	}
	if err := applyRules(t, payload, []string{"subject=title", "payload=body", "href=urls"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["urls"] != "json://localhost" {
		t.Errorf("Apply() = %v, want urls remapped", payload)
	}
}

func TestApplyNested(t *testing.T) {
	// Mirrors the issue-306 use-case from the docs.
	payload := map[string]any{
		"event":     map[string]any{"title": "CPU spike", "state": "critical"},
		"component": map[string]any{"name": "web-server-01"},
	}
	err := applyRules(t, payload, []string{
		"event.title=title", "event.state=type", "component.name=body",
	}, 5)
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["title"] != "CPU spike" || payload["type"] != "critical" || payload["body"] != "web-server-01" {
		t.Errorf("Apply() = %v, want nested values mapped", payload)
	}

	// Deeply nested path (3 levels).
	payload = map[string]any{"a": map[string]any{"b": map[string]any{"c": "deep value"}}}
	if err := applyRules(t, payload, []string{"a.b.c=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "deep value" {
		t.Errorf("Apply() = %v, want body=deep value", payload)
	}
}

func TestApplyNestedFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rule    string
		payload map[string]any
	}{
		{"missing intermediate", "missing.field=body", map[string]any{"other": "data"}},
		{"missing leaf", "event.missing_leaf=title", map[string]any{"event": map[string]any{"state": "ok"}}},
		{"non-dict intermediate", "event.title.extra=body", map[string]any{"event": map[string]any{"title": "flat string"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := applyRules(t, tc.payload, []string{tc.rule}, 5); err == nil {
				t.Errorf("Apply(%q) = nil, want error", tc.rule)
			}
		})
	}

	// Depth guard: "a.b.c" is 3 steps; maxDepth=2 rejects it.
	payload := map[string]any{"a": map[string]any{"b": map[string]any{"c": "too deep"}}}
	if err := applyRules(t, payload, []string{"a.b.c=body"}, 2); err == nil {
		t.Error("Apply(depth 2, 3-step path) = nil, want error")
	}
	if _, ok := payload["body"]; ok {
		t.Error("body written despite depth rejection")
	}
}

func TestApplyNestedNoopTargets(t *testing.T) {
	// Nested source + empty target: resolved, nothing written, nil error.
	payload := map[string]any{"event": map[string]any{"title": "hello"}, "body": "existing"}
	if err := applyRules(t, payload, []string{"event.title="}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "existing" {
		t.Errorf("payload = %v, want body untouched", payload)
	}
	if _, ok := payload["title"]; ok {
		t.Errorf("payload = %v, want no title written", payload)
	}

	// Nested source + non-expected target: silent no-op, nil error.
	payload = map[string]any{"event": map[string]any{"title": "hello"}}
	if err := applyRules(t, payload, []string{"event.title=nonexistent_apprise_field"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if _, ok := payload["nonexistent_apprise_field"]; ok {
		t.Errorf("payload = %v, want no write", payload)
	}
}

func TestApplyArrayIndex(t *testing.T) {
	// Simple index.
	payload := map[string]any{"items": []any{"hello world"}}
	if err := applyRules(t, payload, []string{"items[0]=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "hello world" {
		t.Errorf("payload = %v, want body mapped", payload)
	}

	// Index + subfield (issue #209 example).
	payload = map[string]any{"items": []any{map[string]any{
		"objectURI": "https://example.com/q/1234", "title": "New post",
	}}}
	if err := applyRules(t, payload, []string{"items[0].objectURI=body", "items[0].title=title"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "https://example.com/q/1234" || payload["title"] != "New post" {
		t.Errorf("payload = %v, want items mapped", payload)
	}

	// Later element + chained subscripts.
	payload = map[string]any{"alerts": []any{"a", "b", "critical failure"}}
	if err := applyRules(t, payload, []string{"alerts[2]=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "critical failure" {
		t.Errorf("payload = %v, want third alert", payload)
	}
	payload = map[string]any{"data": []any{[]any{"first", "second"}, []any{"third", "fourth"}}}
	if err := applyRules(t, payload, []string{"data[0][1]=body"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "second" {
		t.Errorf("payload = %v, want chained subscript", payload)
	}
}

func TestApplyArrayDepthGate(t *testing.T) {
	// key[0][1][2].value[3] is 6 steps: default 5 rejects, 6 passes.
	build := func() map[string]any {
		inner := map[string]any{"value": []any{"v0", "v1", "v2", "target"}}
		return map[string]any{"key": []any{[]any{[]any{nil, nil}, []any{nil, nil, inner}}}}
	}
	if err := applyRules(t, build(), []string{"key[0][1][2].value[3]=body"}, 5); err == nil {
		t.Error("Apply(depth 5, 6-step path) = nil, want error")
	}
	payload := build()
	if err := applyRules(t, payload, []string{"key[0][1][2].value[3]=body"}, 6); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "target" {
		t.Errorf("payload = %v, want body=target", payload)
	}
}

func TestApplyArrayFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rule    string
		payload map[string]any
	}{
		{"missing close", "items[0=body", map[string]any{"items": []any{"hello"}}},
		{"stray close", "items0]=body", map[string]any{"items0]": []any{"hello"}}},
		{"stray before open", "a]b[0]=body", map[string]any{"a]b": []any{[]any{"x"}}}},
		{"non-integer", "items[abc]=body", map[string]any{"items": []any{"hello"}}},
		{"out of range", "items[99]=body", map[string]any{"items": []any{"only one"}}},
		{"not indexable", "items[0]=body", map[string]any{"items": map[string]any{"key": "value"}}},
		{"empty segment", "a..b=body", map[string]any{"a": map[string]any{"b": "x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := applyRules(t, tc.payload, []string{tc.rule}, 5); err == nil {
				t.Errorf("Apply(%q) = nil, want error", tc.rule)
			}
		})
	}
}

func TestApplyArrayNoopTargets(t *testing.T) {
	payload := map[string]any{"items": []any{"hello"}, "body": "keep me"}
	if err := applyRules(t, payload, []string{"items[0]="}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if payload["body"] != "keep me" {
		t.Errorf("payload = %v, want body kept", payload)
	}
	payload = map[string]any{"items": []any{"hello"}}
	if err := applyRules(t, payload, []string{"items[0]=nonexistent_field"}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if _, ok := payload["nonexistent_field"]; ok {
		t.Errorf("payload = %v, want no write", payload)
	}
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		key   string
		steps []Step
	}{
		{"title", []Step{{Key: "title"}}},
		{"a.b.c", []Step{{Key: "a"}, {Key: "b"}, {Key: "c"}}},
		{"items[0]", []Step{{Key: "items"}, {Index: 0, IsIndex: true}}},
		{"items[0].objectURI", []Step{{Key: "items"}, {Index: 0, IsIndex: true}, {Key: "objectURI"}}},
		{"a[0][2][2]", []Step{{Key: "a"}, {Index: 0, IsIndex: true}, {Index: 2, IsIndex: true}, {Index: 2, IsIndex: true}}},
		{
			"key[0][2][2].value[3]",
			[]Step{{Key: "key"}, {Index: 0, IsIndex: true}, {Index: 2, IsIndex: true}, {Index: 2, IsIndex: true}, {Key: "value"}, {Index: 3, IsIndex: true}},
		},
		{"items.[0]", []Step{{Key: "items"}, {Index: 0, IsIndex: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			steps, err := ParsePath(tc.key)
			if err != nil {
				t.Fatalf("ParsePath(%q) = %v", tc.key, err)
			}
			if !reflect.DeepEqual(steps, tc.steps) {
				t.Errorf("ParsePath(%q) = %v, want %v", tc.key, steps, tc.steps)
			}
		})
	}
	for _, key := range []string{"items[0", "items0]", "items[abc]", "key[0][abc]", "a]b[0]", "a..b", ".a", "a."} {
		t.Run("bad/"+key, func(t *testing.T) {
			if steps, err := ParsePath(key); err == nil {
				t.Errorf("ParsePath(%q) = %v, want error", key, steps)
			}
		})
	}
}

func TestGetNested(t *testing.T) {
	stepsOf := func(t *testing.T, key string) []Step {
		t.Helper()
		steps, err := ParsePath(key)
		if err != nil {
			t.Fatalf("ParsePath(%q) = %v", key, err)
		}
		return steps
	}
	payload := map[string]any{"a": map[string]any{"b": map[string]any{"c": 42}}}
	if v, ok := GetNested(payload, stepsOf(t, "a.b.c"), "a.b.c"); !ok || v != 42 {
		t.Errorf("GetNested() = %v,%v, want 42,true", v, ok)
	}
	payload = map[string]any{"items": []any{"first", "second", "third"}}
	if v, ok := GetNested(payload, stepsOf(t, "items[1]"), "items[1]"); !ok || v != "second" {
		t.Errorf("GetNested() = %v,%v, want second,true", v, ok)
	}
	if _, ok := GetNested(map[string]any{}, stepsOf(t, "missing"), "missing"); ok {
		t.Error("GetNested(missing) = true, want false")
	}
	if _, ok := GetNested(map[string]any{"items": []any{"only"}}, stepsOf(t, "items[5]"), "items[5]"); ok {
		t.Error("GetNested(oob) = true, want false")
	}
	if _, ok := GetNested(map[string]any{"items": map[string]any{"nested": "dict"}}, stepsOf(t, "items[0]"), "items[0]"); ok {
		t.Error("GetNested(non-list) = true, want false")
	}
}

func TestFromRawQuery(t *testing.T) {
	rules, err := FromRawQuery(":subject=title&:payload=body&tag=x&:href=urls")
	if err != nil {
		t.Fatalf("FromRawQuery() = %v", err)
	}
	want := []Rule{
		{Source: "subject", Target: "title"},
		{Source: "payload", Target: "body"},
		{Source: "href", Target: "urls"},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("FromRawQuery() = %v, want %v", rules, want)
	}

	// Non-':' params are fallback carriers, not rules.
	rules, err = FromRawQuery("tag=family&format=text")
	if err != nil {
		t.Fatalf("FromRawQuery() = %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("FromRawQuery() = %v, want no rules", rules)
	}

	// Missing '=' is a delete rule.
	rules, err = FromRawQuery(":garbage")
	if err != nil {
		t.Fatalf("FromRawQuery() = %v", err)
	}
	if len(rules) != 1 || rules[0].Source != "garbage" || rules[0].Target != "" {
		t.Errorf("FromRawQuery() = %v, want delete rule", rules)
	}

	if _, err := FromRawQuery(":=title"); err == nil {
		t.Error("FromRawQuery(empty source) = nil, want error")
	}
	if _, err := FromRawQuery(":a=%zz"); err == nil {
		t.Error("FromRawQuery(bad escape) = nil, want error")
	}
	if rules, err := FromRawQuery(""); err != nil || rules != nil {
		t.Errorf("FromRawQuery(empty) = %v,%v, want nil,nil", rules, err)
	}
}
