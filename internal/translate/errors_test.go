package translate

import "testing"

// TestClassifyEngineError 验证引擎错误 → 面向用户的类别（issue #42）：
// pair（语言对不可用）/ network（网络不可达）/ auth（认证失败）/ engine（其它）。
// 前端按类别映射可操作的本地化文案；原文细节随事件附带，不参与分类。
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
		{"generic engine error", `引擎返回错误`, "engine"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyEngineError(tc.err); got != tc.want {
				t.Errorf("ClassifyEngineError(%q) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
