package translate

import "testing"

// TestClassifyEngineError verifies engine error → user-facing category (issue #42):
// pair (language pair unavailable) / network (unreachable) / auth (authentication failure) /
// engine (other).
// The frontend maps each category to actionable localized copy; the raw detail travels with
// the event and is not part of the classification.
func TestClassifyEngineError(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want string
	}{
		{"apple pair failure", `[动态桥接] 系统翻译失败: 引擎返回错误 (Unable to Translate)`, "pair"},
		{"unsupported language pair", `unsupported language pair es->zh`, "pair"},
		{"connection refused", `Get "https://x": dial tcp 1.2.3.4:443: connect: connection refused`, "network"},
		{"dns failure", `no such host`, "network"},
		{"client timeout", `Client.Timeout exceeded while awaiting headers`, "network"},
		{"tls handshake", `tls: handshake failure`, "network"},
		{"bad key", `401 Unauthorized: Invalid API key`, "auth"},
		{"forbidden quota", `403 Forbidden`, "auth"},
		{"generic engine error", `some unexpected engine failure`, "engine"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyEngineError(tc.err); got != tc.want {
				t.Errorf("ClassifyEngineError(%q) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
