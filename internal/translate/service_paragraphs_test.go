package translate

import (
	"context"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #144 regression guards (they pass on master by design): the translate window lost line
// breaks in its per-word display, never in the engine path. These pin that a multi-paragraph
// engine result reaches the frontend byte for byte, so a future trim or whitespace collapse in
// the service is caught here and not in a hand test.

const paragraphs = "Para one.\n\nPara two, line one\nline two."

// The one EventTranslateResult payload of a TranslateMulti round carries the engine's string
// exactly: the blank line and the line break included.
func TestTranslateMultiEmitsParagraphsVerbatim(t *testing.T) {
	eng := &fakeEngine{name: "a", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return okResult(paragraphs), nil
	}}
	svc, em, _ := newCancelService(t, eng)
	if _, err := svc.TranslateMulti(multiReq("p-144")); err != nil {
		t.Fatalf("TranslateMulti: %v", err)
	}
	waitUntil(t, "the result payload", func() bool { return len(em.results()) == 1 })
	if got := em.results()[0]; got.Result != paragraphs {
		t.Errorf("Result = %q, want %q (verbatim)", got.Result, paragraphs)
	}
}

// The same through real HTTP: the real google engine against a loopback server serving the gtx
// body captured live on 2026-09-26 (engine.gtxParagraphsBody's twin), through svc.Translate.
func TestTranslateGoogleLoopbackKeepsParagraphs(t *testing.T) {
	const body = `[[["Bonjour le monde. ","Hello, world. ",null,null,10],["Ça marche!\n\n","It works!\n\n",null,null,10],["Deuxième paragraphe ; ","Second paragraph; ",null,null,3],["première ligne\n","line one\n",null,null,3],["ligne deux.","line two.",null,null,3]],null,"en",null,null,null,null,[]]`
	svc := newLoopbackServiceBody(t, body)
	res, err := svc.Translate(model.TranslateRequest{
		Text:       "Hello, world. It works!\n\nSecond paragraph; line one\nline two.",
		From:       model.EN,
		To:         model.FR,
		EngineName: "google",
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	const want = "Bonjour le monde. Ça marche!\n\nDeuxième paragraphe ; première ligne\nligne deux."
	if res.Result != want {
		t.Errorf("Result = %q, want %q", res.Result, want)
	}
}
