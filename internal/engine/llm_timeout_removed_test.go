package engine

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// Issue #109 (criteria 1, 3): the LLM per-request timeout setting and every fixed request limit
// on the eight HTTP engines are gone.

func TestLLMSchemasHaveNoTimeoutField(t *testing.T) {
	for _, name := range []string{"openai", "anthropic", "gemini"} {
		for _, f := range GetEngineSchema(name).Fields {
			if f.Field == "llm_timeout" || string(f.Widget) == "llm_timeout" {
				t.Errorf("%s schema still declares the LLM timeout field: %+v", name, f)
			}
		}
	}
}

// A stored timeout_sec is ignored, not migrated: the row still parses to its model and the parsed
// shape has nowhere to keep a timeout.
func TestParseLLMExtraIgnoresStoredTimeout(t *testing.T) {
	ex := parseLLMExtra(`{"model":"m","timeout_sec":5}`)
	if ex.Model != "m" {
		t.Errorf("Model = %q, want m", ex.Model)
	}
	if _, has := reflect.TypeOf(ex).FieldByName("TimeoutSec"); has {
		t.Error("llmExtra still has a TimeoutSec field; a stored timeout_sec must impose nothing")
	}
}

func TestHTTPEnginesHaveNoFixedRequestLimit(t *testing.T) {
	for _, f := range []string{"google.go", "deepl.go", "baidu.go", "tencent.go", "youdao.go", "openai.go", "anthropic.go", "gemini.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, bad := range []string{"WithTimeout", "Timeout:", "cloneHTTPClientWithTimeout", "timeout time.Duration"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s still contains %q", f, bad)
			}
		}
	}
	b, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"DefaultLLMTimeoutSec", "cloneHTTPClientWithTimeout", "WidgetLLMTimeout"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("engine.go still contains %q", bad)
		}
	}
}
