package translate

import (
	"slices"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #208: what changed is computed in Go, word by word, never by the model.

func TestDiffWords(t *testing.T) {
	cases := []struct {
		name, before, after string
		want                []model.TextChange
	}{
		{"identical", "Hola mundo cruel", "Hola mundo cruel", nil},
		{"whitespace only", "Hola  mundo\ncruel ", "Hola mundo cruel", nil},
		{"one word", "Ellos esta en casa", "Ellos están en casa", []model.TextChange{{Before: "esta", After: "están"}}},
		{"accent only", "el esta aqui", "el está aquí", []model.TextChange{{Before: "esta aqui", After: "está aquí"}}},
		{"case only", "hola mundo", "Hola mundo", []model.TextChange{{Before: "hola", After: "Hola"}}},
		{
			"spanglish",
			"Necesito hacer el follow up con el cliente antes del deadline",
			"Necesito hacer el seguimiento con el cliente antes de la fecha límite",
			[]model.TextChange{{Before: "follow up", After: "seguimiento"}, {Before: "del deadline", After: "de la fecha límite"}},
		},
		// A pure insertion or deletion would read as an empty side: it carries one neighbour.
		{"insertion", "vamos casa", "vamos a casa", []model.TextChange{{Before: "vamos", After: "vamos a"}}},
		{"deletion", "voy de a casa", "voy a casa", []model.TextChange{{Before: "voy de", After: "voy"}}},
		{"insertion at the start", "casa grande", "la casa grande", []model.TextChange{{Before: "casa", After: "la casa"}}},
		{"punctuation", "Hola como estas", "Hola, cómo estás", []model.TextChange{{Before: "como estas", After: ", cómo estás"}}},
		{"empty before", "", "hola", []model.TextChange{{Before: "", After: "hola"}}},
	}
	for _, c := range cases {
		got := diffWords(c.before, c.after)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: diffWords(%q, %q) = %+v, want %+v", c.name, c.before, c.after, got, c.want)
		}
	}
}

func TestDiffWordsChangesReplayToTheCorrectedText(t *testing.T) {
	// Every change's After is in the corrected text and Before in the original: the list is a real
	// description of the two texts, not decoration.
	before := "Yo tiene dos gatos y me gusta mucho los perros"
	after := "Yo tengo dos gatos y me gustan mucho los perros"
	changes := diffWords(before, after)
	if len(changes) == 0 {
		t.Fatal("no changes reported")
	}
	for _, c := range changes {
		if c.Before != "" && !strings.Contains(before, c.Before) {
			t.Errorf("Before %q is not in the original", c.Before)
		}
		if c.After != "" && !strings.Contains(after, c.After) {
			t.Errorf("After %q is not in the corrected text", c.After)
		}
	}
}
