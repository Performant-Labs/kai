package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/settings"
)

// issue #8（Tester 角色，RED）：PrimaryTranslateEngine 的解析规则。
//
// 规则（设计文档 §1，权威实现）：
//   1. settings 的 default_engine 非空、且该引擎在引擎列表中 kind=translate 且 enabled
//      -> 返回它；
//   2. 否则（未设置 / 名字不在列表 / kind 非 translate / 被禁用）
//      -> 回退到引擎列表中第一个 enabled 的 translate 引擎（configstore id 顺序）；
//   3. 没有可用引擎 -> 空串。
//
// 无 mock：引擎配置落 t.TempDir 的真实 SQLite（configstore.Open），google 引擎
// Endpoint 指向真实 loopback httptest.Server，注册进真实 Registry；settings 用
// 真实 settings.Service（真实 settings.json 文件）。app/hotkeyMgr 传 nil：
// 构造函数允许 nil 注入，且本测试只调用 GetEngines 与 PrimaryTranslateEngine，
// 二者均不触碰 app/hotkeyMgr。
//
// RED 说明：EngineWrapper 目前没有 PrimaryTranslateEngine 方法，settings.Settings
// 也没有 DefaultEngine 字段——编译即失败（undefined）。这是「行为缺失」的 RED。
// GetEngines/GetAllEngines 今天都不返回 enabled 状态（GetEngines 按 registry 过滤，
// GetAllEngines 不暴露给解析器），实现时由 PrimaryTranslateEngine 自行按
// configstore 的 enabled 列解析（GetEngines 的 id 顺序语义保持不变）。

const gtxPrimaryFixture = `[[["Hello","Bonjour","","","0"]],null,"en"]`

// setupPrimaryEnv 在 t.TempDir 内搭真实 configstore + settings + registry，
// 按指定引擎行建库并注册。返回 (store, svc, wrapper, cleanup)。
func setupPrimaryEnv(t *testing.T, rows []*engine.EngineConfig) (*configstore.Store, *settings.Service, *EngineWrapper) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("configstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.InitDefaultEngines(ctx, rows); err != nil {
		t.Fatalf("InitDefaultEngines: %v", err)
	}

	svc, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}

	reg := engine.NewRegistry()
	for _, e := range rows {
		if !e.Enabled {
			continue
		}
		switch e.Engine {
		case "google":
			reg.RegisterTranslator(engine.NewGoogle(e.Endpoint, http.DefaultClient))
		case "deepl":
			reg.RegisterTranslator(engine.NewDeepL(e, http.DefaultClient))
		}
	}

	return store, svc, NewEngineWrapper(reg, store, svc, (*application.App)(nil), nil)
}

// TestPrimaryTranslateEngineFallbackInvalidName (b)：default_engine 指向一个
// 不在引擎列表里的名字 -> 解析必须回退到列表中第一个 enabled 的 translate 引擎。
func TestPrimaryTranslateEngineFallbackInvalidName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, svc, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})

	svc.Get().DefaultEngine = "nosuchengine"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := w.PrimaryTranslateEngine(); got != "google" {
		t.Fatalf("invalid default_engine should fall back to the first enabled translate engine google, got %q (PrimaryTranslateEngine missing this fallback)", got)
	}
}

// TestPrimaryTranslateEngineFallsBackWhenPrimaryDisabled (c)：primary 之后被禁用
// -> 解析重新回退到下一个 enabled 的 translate 引擎（不报错、不清空配置）。
func TestPrimaryTranslateEngineFallsBackWhenPrimaryDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	store, svc, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})

	// 设 primary = google（enabled），解析应命中它。
	svc.Get().DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := w.PrimaryTranslateEngine(); got != "google" {
		t.Fatalf("enabled primary should resolve to google, got %q", got)
	}

	// 通过 configstore 真实路径把 primary（google）禁用。
	ctx := context.Background()
	row, err := store.GetEngineByName(ctx, "google")
	if err != nil || row == nil {
		t.Fatalf("GetEngineByName(google): %v", err)
	}
	if err := store.SetEngineEnabled(ctx, row.ID, false); err != nil {
		t.Fatalf("SetEngineEnabled: %v", err)
	}

	// 解析必须重新回退：deepl 是列表里下一个（id 顺序）enabled 的 translate 引擎。
	if got := w.PrimaryTranslateEngine(); got != "deepl" {
		t.Fatalf("disabled primary should re-resolve to deepl, got %q (disabling did not trigger fallback)", got)
	}
}

// TestPrimaryTranslateEngineUnsetFallsBackToFirstEnabled：default_engine 未设置
// （空串，零值）-> 回退到第一个 enabled 的 translate 引擎。规则第 2 步的未设置分支。
func TestPrimaryTranslateEngineUnsetFallsBackToFirstEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, _, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "k"},
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})

	// deepl 在 id 顺序里排第一且 enabled；google 第二。未设置 primary -> deepl。
	if got := w.PrimaryTranslateEngine(); got != "deepl" {
		t.Fatalf("unset default_engine should fall back to the first enabled translate engine deepl, got %q", got)
	}
}
