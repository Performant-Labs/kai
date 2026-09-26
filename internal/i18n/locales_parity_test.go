package i18n

import (
	"encoding/json"
	"sort"
	"testing"
)

// issue #58: en-US.json and zh-CN.json (the merged view shipped via localesFS) must carry the
// same key set. "_comment" is deliberately INCLUDED: both files carry it (the merge script
// always writes it), so it must match too. Nesting is tolerated by flattening to dotted paths.

func flattenKeys(prefix string, v any, out map[string]struct{}) {
	if m, ok := v.(map[string]any); ok {
		for k, child := range m {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flattenKeys(p, child, out)
		}
		return
	}
	out[prefix] = struct{}{}
}

func loadKeys(t *testing.T, name string) map[string]struct{} {
	t.Helper()
	data, err := localesFS.ReadFile("locales/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	out := map[string]struct{}{}
	flattenKeys("", raw, out)
	return out
}

func missingFrom(a, b map[string]struct{}) []string {
	var d []string
	for k := range a {
		if _, ok := b[k]; !ok {
			d = append(d, k)
		}
	}
	sort.Strings(d)
	return d
}

func TestLocaleKeyParity(t *testing.T) {
	en := loadKeys(t, "en-US.json")
	zh := loadKeys(t, "zh-CN.json")
	if len(en) < 50 || len(zh) < 50 {
		t.Fatalf("catalogs suspiciously small (en=%d zh=%d): vacuous parity", len(en), len(zh))
	}
	if d := missingFrom(en, zh); len(d) > 0 {
		t.Errorf("keys in en-US.json missing from zh-CN.json: %v", d)
	}
	if d := missingFrom(zh, en); len(d) > 0 {
		t.Errorf("keys in zh-CN.json missing from en-US.json: %v", d)
	}
}
