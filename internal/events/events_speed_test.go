package events

import (
	"encoding/json"
	"strings"
	"testing"
)

// Issue #10: the started progress event carries what the progress line needs to show a time
// estimate and offer the faster-setting switch. JSON shape test (the frontend reads these tags
// through the generated bindings; internal/translate's shape test covers the wire end to end on
// macOS CI, this one runs everywhere).

func TestTranslateProgressPayloadIssue10Fields(t *testing.T) {
	b, err := json.Marshal(TranslateProgressPayload{
		RequestID:   "r",
		Engine:      "apple",
		Phase:       ProgressPhaseStarted,
		StartedAtMs: 1,
		EstimatedMs: 180000,
		SuggestFast: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, key := range []string{`"estimated_ms":180000`, `"suggest_fast":true`} {
		if !strings.Contains(s, key) {
			t.Errorf("payload JSON %s lacks %s", s, key)
		}
	}
	// Zero values stay out of the wire format (omitempty): a started event for any other
	// engine, and every chunk event, keep their exact pre-#10 shape.
	b, err = json.Marshal(TranslateProgressPayload{RequestID: "r", Engine: "google", Phase: ProgressPhaseStarted})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); strings.Contains(s, "estimated_ms") || strings.Contains(s, "suggest_fast") {
		t.Errorf("payload JSON %s must omit the issue #10 fields when empty", s)
	}
}
