package remap

import "testing"

func TestParse(t *testing.T) {
	r, err := Parse("subject=title")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if r.Source != "subject" || r.Target != "title" {
		t.Errorf("Parse() = %+v, want source=subject target=title", r)
	}
}

func TestParseDelete(t *testing.T) {
	r, err := Parse("existing=")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if r.Source != "existing" || r.Target != "" {
		t.Errorf("Parse() = %+v, want delete rule", r)
	}
}

func TestParseInvalid(t *testing.T) {
	for _, raw := range []string{"", "no-equals", "=title"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) = nil, want error", raw)
		}
	}
}

func TestIsMappableTarget(t *testing.T) {
	for _, name := range []string{"format", "type", "title", "body", "attachment", "tag", "tags", "urls"} {
		if !IsMappableTarget(name) {
			t.Errorf("IsMappableTarget(%q) = false, want true", name)
		}
	}
	if IsMappableTarget("key") {
		t.Error("IsMappableTarget(key) = true, want false (stateful-only)")
	}
}

func TestApplyBasic(t *testing.T) {
	fields := map[string]any{"subject": "hi"}
	r, _ := Parse("subject=title")
	if err := Apply(fields, []Rule{r}, 5); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if err := Apply(fields, nil, 5); err != nil {
		t.Fatalf("Apply(nil rules) = %v", err)
	}
	if err := Apply(fields, []Rule{r}, 0); err == nil {
		t.Error("Apply(depth 0) = nil, want error")
	}
}
