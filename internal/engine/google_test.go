package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// TestGoogleEndpointOverrideAndDetectedLang 验证（issue #11 自动检测的前置契约）：
// 1) endpoint 覆盖生效——请求打到测试服务器（真实 HTTP 回环，无 transport 拦截）；
// 2) gtx 响应第 2 元素（检测源语言）被写入 TranslateResult.From。
func TestGoogleEndpointOverrideAndDetectedLang(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if !strings.Contains(r.URL.RawQuery, "client=gtx") {
			t.Errorf("expected gtx client param, got %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "sl=auto") {
			t.Errorf("expected sl=auto, got %q", r.URL.RawQuery)
		}
		// gtx 形状：[[["dst","src",null,null,N]], null, "检测源语言"]——检测语言在根索引 2。
		_, _ = w.Write([]byte(`[[["Hola","Hello",null,null,1]],null,"es"]`))
	}))
	defer srv.Close()

	g := NewGoogle(srv.URL, srv.Client())
	res, err := g.Translate(context.Background(), model.TranslateRequest{
		Text: "Hello",
		From: model.Language("auto"),
		To:   model.Language("en"),
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if !strings.Contains(gotQuery, "tl=en") {
		t.Errorf("request query = %q, want tl=en", gotQuery)
	}
	if res.Result != "Hola" {
		t.Errorf("Result = %q, want Hola", res.Result)
	}
	if res.From != model.Language("es") {
		t.Errorf("From = %q, want es (detected)", res.From)
	}
	if res.Engine != "google" {
		t.Errorf("Engine = %q, want google", res.Engine)
	}
}
