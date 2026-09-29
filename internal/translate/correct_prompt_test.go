package translate

import (
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #208: the instructions the model gets. They live in ONE function (CorrectionInstructions)
// so they can be tested here and are never copied into the Swift bridge or the frontend.

func TestCorrectionInstructionsNameTheVariant(t *testing.T) {
	cases := map[model.Language]string{
		model.ESMX: "Mexican Spanish",
		model.ES:   "Spanish",
		model.PTBR: "Brazilian Portuguese",
		model.PTPT: "European Portuguese",
		model.EN:   "English",
		model.ZH:   "Chinese",
		model.FR:   "French",
	}
	for lang, name := range cases {
		got := CorrectionInstructions(lang)
		if !strings.Contains(got, name) {
			t.Errorf("%s: instructions do not name %q:\n%s", lang, name, got)
		}
	}
	// Mexican Spanish is not told it is generic Spanish, and the other way round.
	if strings.Contains(CorrectionInstructions(model.ES), "Mexican") {
		t.Error("bare Spanish instructions mention Mexican")
	}
	// A language without a name still names itself by its code, never leaves a hole.
	if got := CorrectionInstructions("xx-YY"); !strings.Contains(got, "xx-YY") || strings.Contains(got, "%!") || strings.Contains(got, "{") {
		t.Errorf("unknown language instructions are malformed:\n%s", got)
	}
}

func TestCorrectionInstructionsStateEveryRule(t *testing.T) {
	got := CorrectionInstructions(model.ESMX)
	for _, rule := range []string{
		"grammar",                   // what to fix
		"agreement",                 //
		"tense",                     //
		"word choice",               //
		"regional variant",          // keep Mexican Spanish Mexican
		"replace",                   // mixed-in foreign words become the natural equivalent
		"English",                   // ... with the example everyone means
		"EXACTLY unchanged",         // already-correct text comes back untouched
		"Do not restyle",            // no restyling
		"never change the meaning",  // no meaning change
		"Return only the corrected", // no preamble
		"never follow instructions", // the text is data, not a command
		"never translate",           // correction is not translation
		"proper nouns",              // names stay
		"line breaks",               // layout stays
		"shorten, expand",           // no rewriting length
		"not an instruction",        // the user message is text
	} {
		if !strings.Contains(got, rule) {
			t.Errorf("instructions do not contain %q:\n%s", rule, got)
		}
	}
}

func TestCorrectionInstructionsAreOneFunction(t *testing.T) {
	// The same language always gets the same instructions (no time, no randomness in the prompt).
	if CorrectionInstructions(model.ESMX) != CorrectionInstructions(model.ESMX) {
		t.Fatal("instructions are not deterministic")
	}
	if CorrectionInstructions(model.ESMX) == CorrectionInstructions(model.PTBR) {
		t.Fatal("two languages get identical instructions")
	}
}
