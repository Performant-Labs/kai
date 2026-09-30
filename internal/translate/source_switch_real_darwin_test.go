//go:build darwin

package translate

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #16: the source switch through the REAL NaturalLanguage detector (the Swift bridge), on
// realistic Mexican Spanish selections. Measured 2026-09-30 on macOS: every sample below scores
// 0.95 or more as es, including Spanish with English words, names, numbers, URLs, line breaks and
// chat text, so the detector's confidence is not what makes Alt-A miss. The samples are invented.
// The negatives pin the hard rule: text that is not in the other language never switches.

func realSwitchService(t *testing.T) *Service {
	t.Helper()
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("Swift bridge not loadable: %v", err)
	}
	svc := NewService(engine.NewRegistry(), nil, nil, nil)
	prefs := langpref.New()
	prefs.Learn(model.ESMX)
	svc.SetLangPrefs(prefs)
	return svc
}

func TestRealDetectorSwitchesMexicanSpanishSelections(t *testing.T) {
	svc := realSwitchService(t)
	for name, text := range map[string]string{
		"twenty code points": "Oye, ¿ya llegaste?!!",
		"short chat":         "Ahorita te marco, estoy en junta",
		"forty":              "Mañana nos vemos en la oficina a las diez",
		"medium":             "Hola equipo, les comparto el resumen de la reunión de hoy. Quedamos en revisar el presupuesto el viernes y mandar la propuesta al cliente antes del lunes.",
		"english tech words": "Necesito que hagas el deploy del branch a staging y me avises cuando el build esté listo",
		"spanglish":          "Bro ya sé, pero el team no quiere hacer el release hasta que el QA termine el testing",
		"names":              "Hola Carlos, dile a María y a Juan que la junta con Google Cloud es a las 3",
		"numbers":            "La junta es el 14/10 a las 15:30, el pedido 48213 llega el lunes 20",
		"url":                "Mira este artículo, está muy bueno: https://www.ejemplo.com/noticias/2026/10/articulo-largo",
		"line breaks":        "Hola a todos,\n\nLes aviso que mañana no hay clases.\n\nSaludos,\nProfe Luis",
		"chat transcript":    "[10:41] Ana: oye ya viste lo de ayer\n[10:41] Luis: no qué pasó\n[10:42] Ana: cambiaron la fecha de la entrega al jueves",
		"bullets":            "- Comprar leche\n- Llamar al doctor\n- Pagar la luz antes del viernes",
		"emoji slang":        "¡Qué onda, wey! 😂😂 ya ni la amuelas, llevo una hora esperándote aquí",
		"half English half":  "Please review the attached document. Por favor revisen el documento adjunto antes de la junta.",
		"no accents":         "no se que hacer con esto ahorita, ya le dije a mi jefe y no contesta",
		"all caps":           "AVISO IMPORTANTE: LA OFICINA ESTARÁ CERRADA EL LUNES",
	} {
		got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: text, From: model.EN, To: model.FR})
		if !got.Switched || got.From != model.ESMX || got.To != model.EN {
			t.Errorf("%s: got %+v, want es-MX -> en", name, got)
		}
	}
}

func TestRealDetectorNeverSwitchesTextThatIsNotSpanish(t *testing.T) {
	svc := realSwitchService(t)
	for name, text := range map[string]string{
		"english with a Spanish word":   "Please tell the señor at the front desk that I will arrive tomorrow morning",
		"english with Spanish names":    "Maria Gonzalez and Carlos Lopez will present the quarterly results tomorrow",
		"english with a Spanish phrase": "She said hasta luego and left the meeting room before the vote started",
		"mostly English chat":           "Hola, can you please send me the report before the meeting tomorrow morning",
		"code":                          "const total = items.reduce((a, b) => a + b.price, 0); // sum the prices",
		"URL only":                      "https://www.ejemplo.com/noticias/2026/10/articulo-largo?id=12345",
		"digits and punctuation":        "12345 67890 !!!! ????? ..... 12345",
		"too short Spanish":             "¿Ya comiste?",
		"nineteen code points":          "Sí, claro, ahorita.",
		"plain English":                 "The quick brown fox jumps over the lazy dog again and again.",
	} {
		got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: text, From: model.EN, To: model.FR})
		if got.Switched {
			t.Errorf("%s switched to %s: %+v", name, got.From, got)
		}
	}
}
