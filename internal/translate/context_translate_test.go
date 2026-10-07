package translate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #48: the user tells the translator it got the context wrong, and the text is translated
// again in the context they gave. RetranslateWithContext is the ONE place that decides which engine
// does it: the selected one when it can follow a context, else (the System / Translation framework
// cannot) the on-device model, else a configured cloud LLM, else an explanation.

// fakeLLM is a translation engine that can also run a prompt (engine.Prompter).
type fakeLLM struct {
	name  string
	mu    sync.Mutex
	out   string
	err   error
	block bool
	calls int
	sys   string
	user  string
}

func (f *fakeLLM) Name() string { return f.name }
func (f *fakeLLM) Translate(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, errors.New("not used")
}
func (f *fakeLLM) Prompt(ctx context.Context, system, user string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.sys, f.user = system, user
	block, out, err := f.block, f.out, f.err
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return "", context.Cause(ctx)
	}
	return out, err
}
func (f *fakeLLM) callCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

// fakeSystem is a plain translation engine: no prompt, like the Translation framework.
type fakeSystem struct{ name string }

func (f *fakeSystem) Name() string { return f.name }
func (f *fakeSystem) Translate(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, errors.New("not used")
}

func newContextService(t *testing.T, fc *fakeCorrector, engines ...engine.Translator) *Service {
	t.Helper()
	reg := engine.NewRegistry()
	for _, e := range engines {
		reg.RegisterTranslator(e)
	}
	svc := NewService(reg, nil, nil, nil)
	if fc != nil {
		svc.corrector = fc
	} else {
		svc.corrector = nil
	}
	return svc
}

func ctxReq(eng string, msgs ...string) model.ContextTranslateRequest {
	return model.ContextTranslateRequest{
		Text: "Necesito la junta de mañana", From: model.ESMX, To: model.EN,
		Engine: eng, Previous: "I need the board of tomorrow", Context: msgs,
	}
}

func TestContextTranslateUsesTheSelectedEngineWhenItCanFollowAContext(t *testing.T) {
	llm := &fakeLLM{name: "openai", out: "  I need tomorrow's meeting  "}
	fc := &fakeCorrector{out: "from the device"}
	svc := newContextService(t, fc, &fakeSystem{name: "apple"}, llm)
	got := svc.RetranslateWithContext(ctxReq("openai", "junta means a meeting"))
	if got.Status != model.ContextTranslateOK || got.Result != "I need tomorrow's meeting" || got.Engine != "openai" || got.Fallback {
		t.Fatalf("got %+v, want ok from openai, no fallback", got)
	}
	if fc.callCount() != 0 {
		t.Error("the on-device model ran although the selected engine could follow the context")
	}
	// The model is told everything it needs, and the user's text is data, not instructions.
	for _, want := range []string{"Necesito la junta de mañana", "I need the board of tomorrow", "junta means a meeting"} {
		if !strings.Contains(llm.user, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, llm.user)
		}
	}
	if !strings.Contains(llm.sys, "Mexican Spanish") || !strings.Contains(llm.sys, "English") {
		t.Errorf("the instructions do not name the languages:\n%s", llm.sys)
	}
}

func TestContextTranslateSystemEngineFallsBackToTheOnDeviceModel(t *testing.T) {
	fc := &fakeCorrector{out: "I need tomorrow's meeting"}
	cloud := &fakeLLM{name: "openai", out: "cloud"}
	svc := newContextService(t, fc, &fakeSystem{name: "apple"}, cloud)
	got := svc.RetranslateWithContext(ctxReq("apple", "junta means a meeting"))
	if got.Status != model.ContextTranslateOK || got.Result != "I need tomorrow's meeting" || !got.Fallback || got.Engine != fc.Name() {
		t.Fatalf("got %+v, want the on-device model as a fallback", got)
	}
	if cloud.callCount() != 0 {
		t.Error("a cloud engine ran although the on-device model could")
	}
	if len(fc.langs) == 0 || fc.langs[0] != model.EN {
		t.Errorf("availability asked for %v, want the target language en", fc.langs)
	}
}

func TestContextTranslateFallsBackToACloudEngineWhenTheDeviceCannot(t *testing.T) {
	fc := &fakeCorrector{status: engine.CorrectionAppleIntelligenceOff}
	cloud := &fakeLLM{name: "anthropic", out: "cloud answer"}
	svc := newContextService(t, fc, &fakeSystem{name: "apple"}, cloud)
	got := svc.RetranslateWithContext(ctxReq("apple", "be formal"))
	if got.Status != model.ContextTranslateOK || got.Engine != "anthropic" || !got.Fallback || got.Result != "cloud answer" {
		t.Fatalf("got %+v, want anthropic as a fallback", got)
	}
}

func TestContextTranslateNothingAvailableSaysWhy(t *testing.T) {
	fc := &fakeCorrector{status: engine.CorrectionAppleIntelligenceOff}
	svc := newContextService(t, fc, &fakeSystem{name: "apple"})
	got := svc.RetranslateWithContext(ctxReq("apple", "be formal"))
	if got.Status != model.ContextTranslateUnavailable || got.Reason != string(engine.CorrectionAppleIntelligenceOff) || got.Result != "" {
		t.Fatalf("got %+v, want unavailable with the device's reason", got)
	}
	svc = newContextService(t, nil, &fakeSystem{name: "apple"})
	got = svc.RetranslateWithContext(ctxReq("apple", "be formal"))
	if got.Status != model.ContextTranslateUnavailable || got.Reason != "none_available" {
		t.Fatalf("no provider at all: got %+v, want none_available", got)
	}
}

func TestContextTranslateNeedsAContextMessage(t *testing.T) {
	llm := &fakeLLM{name: "openai", out: "x"}
	svc := newContextService(t, &fakeCorrector{}, llm)
	for _, msgs := range [][]string{nil, {}, {"", "  \n"}} {
		got := svc.RetranslateWithContext(ctxReq("openai", msgs...))
		if got.Status != model.ContextTranslateNoContext {
			t.Errorf("context %q: Status = %q, want no_context", msgs, got.Status)
		}
	}
	if llm.callCount() != 0 {
		t.Error("an engine was called without any context")
	}
}

func TestContextTranslateFailureAndEmptyAnswer(t *testing.T) {
	svc := newContextService(t, nil, &fakeLLM{name: "openai", err: errors.New("boom: secret text")})
	got := svc.RetranslateWithContext(ctxReq("openai", "be formal"))
	if got.Status != model.ContextTranslateFailed || got.Result != "" || got.Engine != "openai" {
		t.Fatalf("error: got %+v, want failed", got)
	}
	svc = newContextService(t, nil, &fakeLLM{name: "openai", out: "   "})
	if got := svc.RetranslateWithContext(ctxReq("openai", "be formal")); got.Status != model.ContextTranslateFailed {
		t.Fatalf("empty answer: got %+v, want failed", got)
	}
	svc = newContextService(t, &fakeCorrector{err: engine.ErrCorrectionRefused}, &fakeSystem{name: "apple"})
	if got := svc.RetranslateWithContext(ctxReq("apple", "be formal")); got.Status != model.ContextTranslateFailed {
		t.Fatalf("refusal: got %+v, want failed", got)
	}
}

func TestContextTranslateCanBeCancelled(t *testing.T) {
	llm := &fakeLLM{name: "openai", block: true}
	svc := newContextService(t, nil, llm)
	req := ctxReq("openai", "be formal")
	req.RequestID = "ctx-1"
	done := make(chan model.ContextTranslateResult, 1)
	go func() { done <- svc.RetranslateWithContext(req) }()
	deadline := time.After(2 * time.Second)
	for !svc.CancelTranslate("ctx-1", "") {
		select {
		case <-deadline:
			t.Fatal("the request never became cancellable")
		case <-time.After(5 * time.Millisecond):
		}
	}
	select {
	case got := <-done:
		if got.Status != model.ContextTranslateCancelled || got.Result != "" || got.RequestID != "ctx-1" {
			t.Fatalf("got %+v, want cancelled", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the cancel did not reach the engine")
	}
	if n := svc.activeRequestCount(); n != 0 {
		t.Errorf("%d requests still registered after the cancel", n)
	}
}
