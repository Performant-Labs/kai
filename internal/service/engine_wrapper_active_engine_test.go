package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/settings"
)

// issue #9（Tester 角色，RED）：结果区「当前引擎」的服务层解析（设计 §1/§3 测试 (a)）。
//
// 解析链 last-used ?? primary(default_engine) ?? first-enabled：与 #8 的
// PrimaryTranslateEngine 同构，但多一层 last-used（翻译窗口上次使用的引擎，
// 前端持久化在 localStorage，kai:translate:lastEngine）：
//   1. lastUsed 有效（在 configstore 内、kind=translate、enabled 列=1、平台支持）-> 它；
//   2. 否则 default_engine 有效 -> 它（#8 规则第 1 步）；
//   3. 否则第一个 enabled 的 translate 引擎（configstore id 顺序，#8 规则第 2 步）；
//   4. 均无 -> ""。
//
// 权威实现在 EngineWrapper 上必须新增接收 last-used 的入口（本文件按设计命名
// ActiveTranslateEngine）；#8 的 PrimaryTranslateEngine 保持无参、语义不变
// （= last-used 传空串时的退化情形）。
//
// 无 mock：复用同包 #8 的 setupPrimaryEnv（真实 configstore 开 t.TempDir() 的
// config.db 文件、真实 settings.Service、真实 registry；google 的 Endpoint 指向
// 真实 loopback httptest.Server），只借用、不改动。「enabled」状态取自 configstore
// 的 enabled 列（经 store.SetEngineEnabled 落库后解析），不取 registry 的
// Supported——否则 Linux 上 openai（Supported 恒 true）会被误判为可用。
//
// RED 说明：EngineWrapper 目前没有 ActiveTranslateEngine 方法——本文件编译即
// 失败（w.ActiveTranslateEngine undefined）。这是「行为缺失」的 RED，不是
// 环境问题。把本文件移除后 internal/service 包其余测试（含 #8 的
// engine_wrapper_primary_test.go）不受影响、照常通过。

// setupActiveEnv 在 setupPrimaryEnv 之上把 openai 作为新增行插入真实 configstore
// （免 Key、免 Endpoint，默认 disabled）：使「列表里存在一个尚未启用、启用后即可
// 被解析命中、且自增 id 排在已有行之后」的场景成立。InitDefaultEngines 只补缺失
// 行、不启用已有 disabled 行，而 InsertEngineConfig 新增行必然分配更大的 id。
func setupActiveEnv(t *testing.T, rows []*engine.EngineConfig) (*configstore.Store, *settings.Service, *EngineWrapper) {
	t.Helper()
	store, svc, w := setupPrimaryEnv(t, rows)
	if _, err := store.InsertEngineConfig(context.Background(), &engine.EngineConfig{Engine: "openai"}); err != nil {
		t.Fatalf("InsertEngineConfig(openai): %v", err)
	}
	return store, svc, w
}

// 1. last-used 在 enabled 的 translate 引擎中时胜出（覆盖 primary）。
func TestActiveTranslateEngineLastUsedWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})
	svc.Get().DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// last-used = deepl（enabled）-> 应胜出，即使 primary 是 google。
	if got := w.ActiveTranslateEngine("deepl"); got != "deepl" {
		t.Fatalf("last-used deepl should resolve to deepl when among enabled translate engines, got %q (ActiveTranslateEngine missing the last-used precedence layer)", got)
	}
}

// 2. last-used 引擎被禁用后 -> 回退到 primary。
func TestActiveTranslateEngineLastUsedDisabledFallsBackToPrimary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	store, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})
	svc.Get().DefaultEngine = "deepl"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 通过 configstore 真实路径把 last-used（google）禁用。
	ctx := context.Background()
	row, err := store.GetEngineByName(ctx, "google")
	if err != nil || row == nil {
		t.Fatalf("GetEngineByName(google): %v", err)
	}
	if err := store.SetEngineEnabled(ctx, row.ID, false); err != nil {
		t.Fatalf("SetEngineEnabled: %v", err)
	}

	if got := w.ActiveTranslateEngine("google"); got != "deepl" {
		t.Fatalf("disabled last-used should fall back to primary deepl, got %q (disabled last-used not skipped)", got)
	}
}

// 3. last-used 与 primary 都非法 -> 第一个 enabled 的 translate 引擎（id 顺序）。
func TestActiveTranslateEngineAllInvalidFallsBackToFirstEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "k"},
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})
	svc.Get().DefaultEngine = "nosuchengine"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// deepl 在 id 顺序里第一且 enabled；last-used 与 primary 都非法 -> deepl。
	if got := w.ActiveTranslateEngine("nosuchengine2"); got != "deepl" {
		t.Fatalf("when both last-used and primary are invalid, should fall back to the first enabled translate engine deepl, got %q", got)
	}
}

// 4. 全部为空 -> ""（列表里没有任何 enabled 的 translate 引擎）。
func TestActiveTranslateEngineAllEmptyReturnsEmpty(t *testing.T) {
	// 无 google/deepl：setupActiveEnv 补入的 openai 处于 disabled，
	// 因此列表里没有任何 enabled 的 translate 引擎。
	_, _, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "tesseract", Enabled: true},
	})

	if got := w.ActiveTranslateEngine(""); got != "" {
		t.Fatalf("should resolve to \"\" when last-used and default_engine are unset and no translate engine is enabled, got %q", got)
	}
}

//  5. 排序：last-used 与 primary 都未设置时，按 configstore id 顺序取第一个
//     enabled 的 translate 引擎（id 序第一 = deepl；openai 虽也 enabled 但 id 在最后）。
func TestActiveTranslateEngineFirstEnabledIsIDOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	store, _, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "k"},
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})
	// 启用 openai（setupActiveEnv 已插入、id 序最后），使 enabled 集合为
	// {deepl, google, openai}：first-enabled 必须按 id 序命中 deepl。
	ctx := context.Background()
	row, err := store.GetEngineByName(ctx, "openai")
	if err != nil || row == nil {
		t.Fatalf("GetEngineByName(openai): %v", err)
	}
	if err := store.SetEngineEnabled(ctx, row.ID, true); err != nil {
		t.Fatalf("SetEngineEnabled(openai): %v", err)
	}

	if got := w.ActiveTranslateEngine(""); got != "deepl" {
		t.Fatalf("with last-used/primary unset, should pick the first enabled engine by configstore id order (deepl), got %q (looks like alphabetical order or primary precedence)", got)
	}
}

//  6. last-used 指向「存在但未启用」的引擎 -> 回退到 primary（enabled 取自
//     enabled 列而非 registry 的 Supported：openai 在 Linux 上 Supported 恒 true）。
func TestActiveTranslateEngineLastUsedNotEnabledFallsBackToPrimary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	// openai 由 setupActiveEnv 插入且保持 disabled。
	_, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})
	svc.Get().DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := w.ActiveTranslateEngine("openai"); got != "google" {
		t.Fatalf("last-used pointing at a disabled engine should fall back to primary google, got %q (looks like Supported used as enabled)", got)
	}
}
