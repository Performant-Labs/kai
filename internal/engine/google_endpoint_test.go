package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// issue #7（Tester 角色，RED）：google 引擎必须尊重 cfg.Endpoint，
// 留空时回退到当前默认端点 DefaultEndpoint；请求打到真实 loopback
// httptest.Server（无 client/transport mock），gtx 形响应解出
// TranslateResult.From。

// gtxFixture 是一条 Google /translate_a/single?client=gtx 响应形，匹配
// googleResponse.UnmarshalJSON 的读取契约：
//
//	根[0] = 翻译段数组，每段 [dst, src, null, null, N]，dst 在 [0]；
//	根[2] = 检测出的源语言（detectedLang）。
//
// 译文 = 各段 dst 拼接 = "Hello world!"。
const gtxFixture = `[[["Hello","Bonjour","","","0"],[" world!","le monde","","","1"]],null,"en"]`

// startGtxServer 起一个 loopback httptest.Server，把收到的 query 参数记进
// got（*url.Values），并回 gtx 形响应（detected lang = "en"，译文 "Hello world!"）。
func startGtxServer(t *testing.T, got *url.Values) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("client") != "gtx" {
			http.Error(w, "missing client=gtx", http.StatusBadRequest)
			return
		}
		if got != nil {
			*got = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxFixture))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestGoogleHonorsConfiguredEndpoint：cfg.Endpoint 指向 loopback 时，
// NewGoogle 必须用它构造引擎，Translate 的真实请求打到该 server
// （若引擎忽略 endpoint，请求会去 https://translate.googleapis.com，
// 而 loopback 收不到 → 断言失败）。TranslateResult.From 必须来自
// gtx 响应的检测语言 "en"。
func TestGoogleHonorsConfiguredEndpoint(t *testing.T) {
	var got url.Values
	srv := startGtxServer(t, &got)

	cfg := &EngineConfig{Endpoint: srv.URL}
	tr := NewGoogle(cfg.Endpoint, http.DefaultClient)
	if tr == nil {
		t.Fatal("NewGoogle returned nil")
	}

	res, err := tr.Translate(context.Background(), model.TranslateRequest{
		Text: "Bonjour le monde",
		From: model.Auto,
		To:   model.EN,
	})
	if err != nil {
		t.Fatalf("Translate failed (engine may not honor cfg.Endpoint; request never reached the loopback): %v", err)
	}
	if got == nil {
		t.Fatal("loopback server received no requests: engine did not use cfg.Endpoint")
	}
	if q := got.Get("sl"); q != "auto" {
		t.Errorf("gtx request sl should be auto (source language auto), got %q", q)
	}
	if q := got.Get("tl"); q != "en" {
		t.Errorf("gtx request tl should be en, got %q", q)
	}
	if q := got.Get("q"); q != "Bonjour le monde" {
		t.Errorf("gtx request q should echo the source text, got %q", q)
	}
	if res.From != model.EN {
		t.Errorf("TranslateResult.From should come from the gtx response's detected language en, got %q", res.From)
	}
	if res.Result != "Hello world!" {
		t.Errorf("translated text should be Hello world!, got %q", res.Result)
	}
	if res.Engine != "google" {
		t.Errorf("Engine should be google, got %q", res.Engine)
	}
	if res.To != model.EN {
		t.Errorf("To should stay the requested en, got %q", res.To)
	}
}

// TestGoogleEmptyEndpointFallsBackToDefault：cfg.Endpoint 留空时，
// NewGoogle 必须回退到当前默认端点（engine.DefaultEndpoint）。
// 这里不发起真实请求（那需要外网），只验证回退后的引擎内部
// endpoint 与 DefaultEndpoint 一致。
func TestGoogleEmptyEndpointFallsBackToDefault(t *testing.T) {
	// 空 Endpoint：cfg.Endpoint == ""，NewGoogle 必须回退到 DefaultEndpoint。
	cfg := &EngineConfig{}
	tr := NewGoogle(cfg.Endpoint, http.DefaultClient)
	g, ok := tr.(*googleTranslator)
	if !ok {
		t.Fatalf("NewGoogle should return *googleTranslator, got %T", tr)
	}
	if g.endpoint != DefaultEndpoint {
		t.Errorf("empty cfg.Endpoint should fall back to default endpoint %s, got %q", DefaultEndpoint, g.endpoint)
	}
}

// TestGoogleZHCodeNormalisation：gtx 端点对 "zh" 的稳定性要求，
// googleLang 必须把 zh / zh-CN / zh_CN 统一成 zh-CN 发到 sl。
// 通过 loopback 捕获真实请求参数验证（不走 transport mock）。
func TestGoogleZHCodeNormalisation(t *testing.T) {
	for _, from := range []model.Language{model.ZH} {
		t.Run(string(from), func(t *testing.T) {
			var got url.Values
			srv := startGtxServer(t, &got)
			cfg := &EngineConfig{Endpoint: srv.URL}
			tr := NewGoogle(cfg.Endpoint, http.DefaultClient)
			if _, err := tr.Translate(context.Background(), model.TranslateRequest{
				Text: "hello",
				From: from,
				To:   model.EN,
			}); err != nil {
				t.Fatalf("Translate failed: %v", err)
			}
			if got == nil {
				t.Fatal("loopback server received no requests")
			}
			if q := got.Get("sl"); q != "zh-CN" {
				t.Errorf("sl should be normalized to zh-CN, got %q", q)
			}
		})
	}
}
