package translate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #109: the wire shapes the frontend relies on (criteria 4, 10).
func TestTranslateProgressPayloadJSONShape(t *testing.T) {
	b, err := json.Marshal(events.TranslateProgressPayload{RequestID: "r", Engine: "e", Phase: "chunk", StartedAtMs: 7, Done: 1, Total: 2})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"request_id", "engine", "phase", "started_at_ms", "done", "total"} {
		if _, ok := m[k]; !ok {
			t.Errorf("progress payload JSON %s lacks key %q", b, k)
		}
	}
	if events.EventTranslateProgress != "kai:translate:progress" {
		t.Errorf("EventTranslateProgress = %q", events.EventTranslateProgress)
	}
}

func TestTranslateResultCancelledAndRequestIDJSONShape(t *testing.T) {
	b, _ := json.Marshal(model.TranslateResult{Engine: "e", RequestID: "r", Cancelled: true})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["request_id"] != "r" || m["cancelled"] != true {
		t.Errorf("cancelled payload JSON = %s", b)
	}
	if _, ok := m["error"]; ok {
		t.Errorf("cancelled payload carries an error key: %s", b)
	}
	b, _ = json.Marshal(model.TranslateResult{Engine: "e"})
	if strings.Contains(string(b), "cancelled") || strings.Contains(string(b), "request_id") {
		t.Errorf("a plain result must omit cancelled/request_id (omitempty): %s", b)
	}
	b, _ = json.Marshal(model.TranslateRequest{Text: "x", RequestID: "r"})
	if !strings.Contains(string(b), `"request_id":"r"`) {
		t.Errorf("TranslateRequest JSON %s lacks request_id", b)
	}
	b, _ = json.Marshal(model.TranslateMultiResult{Count: 1, RequestID: "r", Engines: []string{"a"}})
	if !strings.Contains(string(b), `"engines":["a"]`) || !strings.Contains(string(b), `"request_id":"r"`) {
		t.Errorf("TranslateMultiResult JSON = %s", b)
	}
	b, _ = json.Marshal(model.ScreenshotResult{RequestID: "r"})
	if !strings.Contains(string(b), `"request_id":"r"`) {
		t.Errorf("ScreenshotResult JSON = %s", b)
	}
}

// Criteria 1 and 17: the service has no translation deadline left, and the copy for the deadline
// branch is gone from every catalog.
func TestServiceHasNoTranslationDeadlineOrTimeoutCopy(t *testing.T) {
	src, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if strings.Contains(s, "timeout := 30 * time.Second") {
		t.Error("translateWithEngine still builds a 30 s deadline")
	}
	if n := strings.Count(s, "WithTimeout"); n != 3 {
		t.Errorf("service.go has %d WithTimeout calls, want exactly 3 (region capture, OCR, history write)", n)
	}
	if strings.Contains(s, "err.translate_timeout") {
		t.Error("service.go still references err.translate_timeout")
	}
	for _, f := range []string{
		"../i18n/locales/en-US.json", "../i18n/locales/zh-CN.json",
		"../i18n/locales/split/en-US/errors.json", "../i18n/locales/split/zh-CN/errors.json",
	} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "err.translate_timeout") {
			t.Errorf("%s still carries err.translate_timeout", f)
		}
	}
}
