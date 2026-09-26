package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// closedLoopbackURL returns an http URL whose port has nothing listening (a server that was
// started and closed), so any request fails at connect time with a transport error.
func closedLoopbackURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	return u
}

// Issue #96 AC4 (E6): every HTTP engine wraps its transport cause (errors.As net.Error) and no
// engine error text carries a "%!(" formatting artifact (doubled args left over from a
// one-verb i18n string).
func TestEngineTransportFailureWrapsCauseAndHasNoFormatArtifacts(t *testing.T) {
	hc := &http.Client{Timeout: 20 * time.Second}
	req := model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH}

	builders := map[string]func(ep string) Translator{
		"google": func(ep string) Translator { return NewGoogle(ep, hc) },
		"deepl": func(ep string) Translator {
			return NewDeepL(&EngineConfig{Engine: "deepl", APIKey: "k-deepl", Endpoint: ep}, hc)
		},
		"openai": func(ep string) Translator {
			return NewOpenAI(&EngineConfig{Engine: "openai", APIKey: "k-openai", Endpoint: ep}, hc)
		},
		"anthropic": func(ep string) Translator {
			return NewAnthropic(&EngineConfig{Engine: "anthropic", APIKey: "k-anthropic", Endpoint: ep, HTTPClient: hc}) //nolint:gosec // G101: fake key for the redaction test, not a real credential
		},
		"gemini": func(ep string) Translator {
			g, err := NewGemini(&EngineConfig{Engine: "gemini", APIKey: "k-gemini", Endpoint: ep, HTTPClient: hc})
			if err != nil {
				t.Fatalf("NewGemini: %v", err)
			}
			return g
		},
		"baidu": func(ep string) Translator {
			return NewBaidu(&EngineConfig{Engine: "baidu", APIKey: "appid", Secret: "s-baidu", Endpoint: ep}, hc)
		},
		"tencent": func(ep string) Translator {
			return NewTencent(&EngineConfig{Engine: "tencent", APIKey: "id", Secret: "s-tencent", Endpoint: ep}, hc)
		},
		"youdao": func(ep string) Translator {
			return NewYoudao(&EngineConfig{Engine: "youdao", APIKey: "appkey", Secret: "s-youdao", Endpoint: ep}, hc)
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			tr := build(closedLoopbackURL(t))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := tr.Translate(ctx, req)
			if err == nil {
				t.Fatal("Translate against a closed port returned no error")
			}
			if strings.Contains(err.Error(), "%!(") {
				t.Errorf("error text has a format artifact: %q", err.Error())
			}
			var ne net.Error
			if !errors.As(err, &ne) {
				t.Errorf("errors.As(net.Error) = false; cause not wrapped: %v", err)
			}
		})
	}
}

// Issue #96 AC7 (E9): the unsupported-target refusal wraps ErrUnsupportedPair; its text is
// byte-identical to today's.
func TestUnsupportedTargetErrorWrapsPairSentinelTextUnchanged(t *testing.T) {
	l := model.Language("xx")
	err := unsupportedTargetError("google", l)
	if !errors.Is(err, ErrUnsupportedPair) {
		t.Errorf("errors.Is(err, ErrUnsupportedPair) = false: %v", err)
	}
	want := i18n.T("err.engine_unsupported_target", "engine", "google", "lang", languageLabel(l))
	if err.Error() != want {
		t.Errorf("text = %q, want unchanged %q", err.Error(), want)
	}
}

// Issue #96 AC8: DeepL's and Anthropic's in-engine missing-key paths wrap ErrAPIKey with their
// existing text, and make no network request.
func TestMissingKeyPathsWrapAPIKeySentinel(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "should not be called", http.StatusTeapot)
	}))
	defer srv.Close()
	req := model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH}

	cases := []struct {
		name     string
		tr       Translator
		wantText string
	}{
		{"deepl", NewDeepL(&EngineConfig{Engine: "deepl", Endpoint: srv.URL}, srv.Client()), i18n.T("err.deepl_missing_apikey")},
		{"anthropic", NewAnthropic(&EngineConfig{Engine: "anthropic", Endpoint: srv.URL, HTTPClient: srv.Client()}), i18n.T("err.anthropic_missing_apikey")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.tr.Translate(context.Background(), req)
			if !errors.Is(err, ErrAPIKey) {
				t.Fatalf("errors.Is(err, ErrAPIKey) = false: %v", err)
			}
			if err.Error() != tc.wantText {
				t.Errorf("text = %q, want unchanged %q", err.Error(), tc.wantText)
			}
		})
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server hits = %d, want 0 (no request without a key)", n)
	}
}

// fakeEngine is a configurable Translator for the WithSecrets tests.
type fakeEngine struct {
	name string
	res  *model.TranslateResult
	err  error
}

func (f *fakeEngine) Name() string { return f.name }
func (f *fakeEngine) Translate(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
	return f.res, f.err
}

// noAutoEngine additionally declares it does not support auto-detect (the optional interface).
type noAutoEngine struct{ fakeEngine }

func (noAutoEngine) SupportsAutoSource() bool { return false }

// Issue #96 AC9(a): WithSecrets replaces the literal configured secrets with *** in Error(), keeps
// the chain (errors.As still finds the *HTTPError), delegates Name(), forwards the optional
// SupportsAutoSource interface, ignores empty secrets, and passes successes through.
func TestWithSecretsRedactsAndKeepsChain(t *testing.T) {
	// Assembled at run time so no literal key-shaped string is committed (secret scan); not real credentials.
	key, sec := "sk-"+"live-abcdef123456", "s3cr3t-"+"value"
	inner := &fakeEngine{name: "fake", err: fmt.Errorf("call failed key=%s secret=%s: %w", key, sec, &HTTPError{Status: 401, Message: "bad " + key})}
	w := WithSecrets(inner, key, sec, "")
	_, err := w.Translate(context.Background(), model.TranslateRequest{Text: "x"})
	if err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(err.Error(), key) || strings.Contains(err.Error(), sec) {
		t.Errorf("secret leaked in Error(): %q", err.Error())
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("Error() = %q, want *** in place of the secrets", err.Error())
	}
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 401 {
		t.Errorf("errors.As(*HTTPError) failed or wrong status: %v", err)
	}
	if w.Name() != "fake" {
		t.Errorf("Name() = %q, want delegated fake", w.Name())
	}
}

func TestWithSecretsEmptySecretDoesNotMangleText(t *testing.T) {
	inner := &fakeEngine{name: "fake", err: errors.New("plain failure")}
	_, err := WithSecrets(inner, "").Translate(context.Background(), model.TranslateRequest{Text: "x"})
	if err == nil || err.Error() != "plain failure" {
		t.Errorf("err = %v, want unchanged plain failure", err)
	}
}

func TestWithSecretsPassesSuccessThrough(t *testing.T) {
	want := &model.TranslateResult{Engine: "fake", Result: "ok"}
	got, err := WithSecrets(&fakeEngine{name: "fake", res: want}, "k").Translate(context.Background(), model.TranslateRequest{Text: "x"})
	if err != nil || got != want {
		t.Errorf("got (%v, %v), want the inner result untouched", got, err)
	}
}

func TestWithSecretsForwardsAutoSourceSupport(t *testing.T) {
	if !SupportsAutoSource(WithSecrets(&fakeEngine{name: "fake"}, "k")) {
		t.Error("wrapper of an engine with no opinion must support auto source (default true)")
	}
	if SupportsAutoSource(WithSecrets(&noAutoEngine{fakeEngine{name: "fake"}}, "k")) {
		t.Error("wrapper hid the inner engine's SupportsAutoSource()=false")
	}
}
