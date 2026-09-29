package engine

import (
	"bytes"
	"encoding/json"
	"strings"

	"cnb.cool/dtapp/kai/internal/model"
)

// Local language detection (issue #200). The translate service decides whether text that arrived
// in another language than the pinned source should switch the dropdowns, and it needs the
// language before any engine has run. On macOS that is NaturalLanguage's recognizer through the
// Swift bridge, a synchronous computation of about a millisecond once warm (measured, see the
// #200 PR), with a confidence. Elsewhere there is no local detector: DetectLanguage reports none,
// and the service falls back to the detection a translation result carries (issue #161).

// detectionPayload is the bridge's answer, mirroring swiftbridge.DetectedLanguage. It is declared
// here as well so the parse is testable on every OS, without the darwin-only bridge package.
type detectionPayload struct {
	Lang       string  `json:"lang"`
	Confidence float64 `json:"confidence"`
}

// parseDetection reads the bridge's JSON. Nothing that is not a usable detection comes back as
// one: a malformed or empty payload, a bridge error object, an empty language and a confidence
// outside [0, 1] are all "no detection", so a broken answer can never switch anything. The
// language is returned as the bridge reports it, the bare code (es, zh); mapping it to a language
// of the app is the caller's (model.ParseLanguage).
func parseDetection(raw []byte) (model.Language, float64, bool) {
	raw = bytes.TrimRight(raw, "\x00")
	var p detectionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", 0, false
	}
	lang := strings.TrimSpace(p.Lang)
	if lang == "" || strings.EqualFold(lang, "und") || p.Confidence < 0 || p.Confidence > 1 {
		return "", 0, false
	}
	return model.Language(lang), p.Confidence, true
}
