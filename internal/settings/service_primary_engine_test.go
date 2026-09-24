package settings

import (
	"testing"
)

// issue #8（Tester 角色，RED）：default_engine 配置字段的落盘往返。
//
// 真实数据目录（t.TempDir）+ 真实 settings.json 文件，无 mock：
// 保存后换新 Service 实例重新读盘，default_engine 必须原样往返。
//
// RED 说明：当前 Settings 结构体没有 DefaultEngine 字段（json "default_engine"），
// writeConfig/setDefaults 也不序列化它，因此本文件编译即失败
// （cfg.DefaultEngine undefined）。这是「行为缺失」的 RED，不是环境或拼写问题。

func TestDefaultEngineConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()

	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	cfg := svc.Get()
	cfg.DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 新实例重新读盘：default_engine 必须从 settings.json 往返回来。
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatalf("第二次 NewService: %v", err)
	}
	if got := svc2.Get().DefaultEngine; got != "google" {
		t.Fatalf("default_engine 落盘往返失败：期望 %q，得到 %q（settings 服务未持久化 default_engine）", "google", got)
	}
}
