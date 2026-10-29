package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// baiduMultilineBody is Baidu's documented response shape for a q containing newlines: one
// trans_result entry (src/dst pair) per newline-separated segment (general translation API documentation,
// https://fanyi-api.baidu.com/doc/21, FAQ 1): a q containing newlines returns an independent
// translation result for each segment of text. The empty middle entry for the blank source line is an inference from
// that one-entry-per-segment rule (each segment, including an empty one, contributes one entry),
// consistent with real-world clients that index one dst per source line. NOT captured live: no
// Baidu credentials exist in this run, so the fixture is built from the documented shape.
const baiduMultilineBody = `{"from":"en","to":"zh","trans_result":[{"src":"First line.","dst":"第一行。"},{"src":"","dst":""},{"src":"Third line.","dst":"第三行。"}]}`

// TestBaiduMultilineResultJoinsAllLines (issue #146): a multi-line q gets one trans_result entry
// per line, so Result must carry every dst in order, "\n"-joined, with blank lines preserved.
// RED contract: today's baidu.go returns only TransResult[0].Dst, so this test fails at RED on the
// joined-Result assertion; the request side already sends the raw multi-line q (form.Set("q",
// req.Text)), which the posted-form check pins.
func TestBaiduMultilineResultJoinsAllLines(t *testing.T) {
	const src = "First line.\n\nThird line."
	var gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotQ = r.PostForm.Get("q")
		_, _ = w.Write([]byte(baiduMultilineBody))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{Engine: "baidu", APIKey: "appid", Secret: "s-baidu", Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(context.Background(), model.TranslateRequest{
		Text: src,
		From: model.EN,
		To:   model.ZH,
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if gotQ != src {
		t.Errorf("posted q = %q, want the exact multi-line source %q", gotQ, src)
	}
	const want = "第一行。\n\n第三行。"
	if res.Result != want {
		t.Errorf("Result = %q, want %q (all lines in order, blank line preserved)", res.Result, want)
	}
}

// TestBaiduSingleEntryResultUnchanged (guard): a single-entry response (the no-newline case)
// keeps Result byte-identical to today's TransResult[0].Dst — no stray join artifacts — and the
// result fields stay as today. This test PASSES at RED by design (single-entry behavior already
// works) and must keep passing after the fix.
func TestBaiduSingleEntryResultUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"from":"en","to":"zh","trans_result":[{"src":"Hello","dst":"你好"}]}`))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{Engine: "baidu", APIKey: "appid", Secret: "s-baidu", Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(context.Background(), model.TranslateRequest{
		Text: "Hello",
		From: model.EN,
		To:   model.ZH,
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if res.Result != "你好" {
		t.Errorf("Result = %q, want the single dst unchanged", res.Result)
	}
	if res.Engine != "baidu" {
		t.Errorf("Engine = %q, want baidu", res.Engine)
	}
}
