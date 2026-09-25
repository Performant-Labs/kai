package translate

import "strings"

// 引擎错误类别（issue #42）：前端按类别映射可操作的本地化文案，
// 原始错误细节随 TranslateResult.Error 附带，不参与分类。
const (
	ErrorKindPair    = "pair"    // 语言对不可用（如 macOS 系统翻译未下载该语言对）
	ErrorKindNetwork = "network" // 网络/端点不可达（超时、DNS、连接拒绝、TLS）
	ErrorKindAuth    = "auth"    // 认证/授权失败（401/403、无效 API key）
	ErrorKindEngine  = "engine"  // 其它引擎错误（兜底）
)

// ClassifyEngineError 把引擎返回的错误文本归类为面向用户的类别。
// 只做子串匹配——各类引擎的错误文案不统一，精确解析不现实；
// 分类错误的代价是多显示一条笼统文案（engine 兜底），可接受。
func ClassifyEngineError(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case containsAny(lower,
		"unable to translate",
		"language pair",
		"unsupported language",
		"pair not",
	):
		return ErrorKindPair
	case containsAny(lower,
		"timeout",
		"connection refused",
		"connection reset",
		"no such host",
		"dial tcp",
		"network is unreachable",
		"tls",
		"proxy",
	):
		return ErrorKindNetwork
	case containsAny(lower,
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid api key",
		"invalid key",
		"authentication",
	):
		return ErrorKindAuth
	default:
		return ErrorKindEngine
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
