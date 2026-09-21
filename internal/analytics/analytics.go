// Package analytics 提供统一的匿名使用统计上报（基于 PostHog）。
//
// 设计要点：
//   - 单一上报出口：翻译/OCR/更新等后端事件直接在 Go 侧调用 Track；
//     纯前端 UI 事件（设置页打开、开关切换）由前端经 Wails 事件 kai:analytics:track 转交 Track，
//     因此不需要在前端 webview 里加载 posthog-js，前后端共用同一个匿名设备 ID。
//   - 门控：仅当 buildinfo.PosthogToken 非空 + 非 dev 构建（或显式 EnableDevUpload）+ 用户开启开关（settings.analytics_enabled）时才上报。
//   - 匿名：设备 ID 取自 machineid.ProtectedID("kai")（按应用名 HMAC 的机器指纹，匿名且稳定）；
//     获取失败则不上报。不收集任何用户信息。
//   - 隐私：开关关闭后立刻关闭 client、停止连网。
package analytics

import (
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
	"github.com/denisbrodbeck/machineid"
	"github.com/posthog/posthog-go"
)

// PostHog Token 统一由 buildinfo.PosthogToken 经 -ldflags 注入，不再读取任何环境变量。

// endpoint PostHog 上报地址（云版 US 默认；自托管改成你们的实例地址）。
const endpoint = "https://us.i.posthog.com"

// appName 上报时携带的应用标识（固定为 kai，用于在同一 PostHog 实例多应用/多构建时区分）。
const appName = "kai"

// 事件名常量（与产品分析方案对齐）。
const (
	EventAppStarted          = "app_started"
	EventAppInstalled        = "app_installed"
	EventTranslateInput      = "translate_input"
	EventTranslateScreenshot = "translate_screenshot"
	EventSettingsOpened      = "settings_opened"
	EventFeatureToggled      = "feature_toggled"
	EventEngineConfigured    = "engine_configured"
	EventUpdateInstalled     = "update_installed"
	EventError               = "error_occurred"
)

var (
	mu        sync.RWMutex
	client    posthog.Client
	hasClient bool
	deviceID  string
	deviceOK  bool
	ss        *settings.Service
)

// devUpload 允许 dev 构建（Dev=true）也上报数据的本地调试开关，默认 false。
// 正式构建（Dev=false）不受此开关影响，始终按 token + 用户开关决定。
var devUpload atomic.Bool

// EnableDevUpload 打开 dev 构建下的上报，仅用于本地调试验证管道。
// 调用后 IsDev() 不再拦截，行为等同正式构建。生产环境切勿调用。
func EnableDevUpload() {
	devUpload.Store(true)
}

// devModeBlocks 当前是否处于“禁止上报”的 dev 态：dev 构建且未显式开启 dev 上报。
// 所有上报门控统一走此方法，便于日后调整 dev 测试策略。
func devModeBlocks() bool {
	return buildinfo.IsDev() && !devUpload.Load()
}

// Init 惰性初始化：获取匿名设备 ID（machineid），记录 settings 服务。
// 必须在 settings.NewService 之后、app.Run 之前调用。
// 若 machineid 不可用，deviceOK=false，后续上报将被 Enabled 拦截（错误即不上报）。
func Init(dataDir string, svc *settings.Service) {
	mu.Lock()
	defer mu.Unlock()
	ss = svc
	id, err := machineid.ProtectedID("kai")
	if err != nil {
		slog.Warn(i18n.T("log.analytics_device_id_failed"), "error", err)
		deviceID = ""
		deviceOK = false
		return
	}
	deviceID = id
	deviceOK = true
}

// Enabled 当前是否允许上报：token 非空 + machineid 可用 + 非 dev 构建 + 用户开启开关。
func Enabled() bool {
	if buildinfo.PosthogToken == "" {
		return false
	}
	if !deviceOK {
		return false
	}
	if devModeBlocks() {
		return false
	}
	mu.RLock()
	defer mu.RUnlock()
	return ss != nil && ss.Get() != nil && ss.Get().AnalyticsEnabled
}

// analyticsState 返回当前是否上报及原因（i18n key），用于启动日志（不直接用 Enabled()
// 是为了在关闭时也能给出“为什么关”的提示：未配 token / 设备 ID 不可用 / dev 构建 / 用户关闭）。
// 返回的是 i18n key，由调用方经 i18n.T 翻译，避免硬编码文案。
func analyticsState() (enabled bool, reasonKey string) {
	if buildinfo.PosthogToken == "" {
		return false, "log.analytics_reason_no_token"
	}
	if !deviceOK {
		return false, "log.analytics_reason_no_device"
	}
	if devModeBlocks() {
		return false, "log.analytics_reason_dev"
	}
	mu.RLock()
	on := ss != nil && ss.Get() != nil && ss.Get().AnalyticsEnabled
	mu.RUnlock()
	if !on {
		return false, "log.analytics_reason_user_off"
	}
	return true, "log.analytics_reason_on"
}

// IsFirstLaunch 是否尚未发送过 app_installed（首装事件仅发一次）。
// 基于 settings 中持久化的 AnalyticsInstalled 标志；调用前需已 Init。
func IsFirstLaunch() bool {
	mu.RLock()
	svc := ss
	mu.RUnlock()
	return svc != nil && svc.Get() != nil && !svc.Get().AnalyticsInstalled
}

// ensureClient 按需建立 client（仅当 key 非空且非 dev）。已建立则直接返回。
func ensureClient() posthog.Client {
	mu.RLock()
	if hasClient {
		c := client
		mu.RUnlock()
		return c
	}
	mu.RUnlock()

	key := buildinfo.PosthogToken
	if key == "" || devModeBlocks() {
		return nil
	}
	// 关键：必须自传 Transport。posthog-go 的 makeHttpClient 仅在 transport==nil 时
	// 才会去断言 http.DefaultTransport.(*http.Transport)，而本项目已把 DefaultTransport
	// 替换为 *useragent.Transport（含 UA/日志包裹层），断言失败直接 panic。主动传入非 nil
	// 的 Transport（即项目已包裹好的 DefaultTransport），它就不会触碰默认 transport，
	// 上报请求同样带上全局 UA 并计入 HTTP 日志。
	//
	// DisableGeoIP：该 SDK 默认（nil）会禁用 GeoIP 归属（GetDisableGeoIP 在 nil 时返回 true，
	// 自动给每事件带 $geoip_disable）。Kai 是桌面客户端，需显式 Ptr(false) 放开，
	// 让 PostHog 按请求 IP 做粗粒度地理归属（国家/地区/城市，不含精确位置）。
	// IsServer：默认（nil）会把事件标为 server-side，而 server 事件 PostHog 不会用请求 IP
	// 做 GeoIP 归因；Kai 实际运行在用户机器上，应 Ptr(false) 以客户端身份上报，
	// 否则 $is_server 会被省略且设备 OS 归属正常。
	c, err := posthog.NewWithConfig(key, posthog.Config{
		Endpoint:               endpoint,
		Transport:              http.DefaultTransport,
		DisableGeoIP:           new(false),
		IsServer:               new(false),
		DefaultEventProperties: posthog.NewProperties().Set("app_name", appName),
	})
	if err != nil {
		slog.Warn(i18n.T("log.analytics_init_client_failed"), "error", err)
		return nil
	}
	mu.Lock()
	client = c
	hasClient = true
	mu.Unlock()
	return c
}

// Track 上报一个自定义事件。内部已做开关/构建模式门控，调用方无需判断。
func Track(event string, props map[string]any) {
	if !Enabled() {
		return
	}
	c := ensureClient()
	if c == nil {
		return
	}
	p := posthog.NewProperties()
	for k, v := range props {
		p.Set(k, v)
	}
	if err := c.Enqueue(posthog.Capture{DistinctId: deviceID, Event: event, Properties: p}); err != nil {
		slog.Warn(i18n.T("log.analytics_track_failed"), "event", event, "error", err)
	}
}

// Identify 设置设备级 person 属性（版本/系统/语言等），使后续事件自动继承。
func Identify(props map[string]any) {
	if !Enabled() {
		return
	}
	c := ensureClient()
	if c == nil {
		return
	}
	p := posthog.NewProperties()
	for k, v := range props {
		p.Set(k, v)
	}
	// app_name 固定标识，作为 person 属性便于在同一 PostHog 实例多应用/多构建时区分。
	p.Set("app_name", appName)
	// project_id 来自 buildinfo 注入，作为 person 属性便于在同一 PostHog 实例多项目时分组。
	if buildinfo.PosthogProjectID != "" {
		p.Set("project_id", buildinfo.PosthogProjectID)
	}
	if err := c.Enqueue(posthog.Identify{DistinctId: deviceID, Properties: p}); err != nil {
		slog.Warn(i18n.T("log.analytics_identify_failed"), "error", err)
	}
}

// Error 便捷上报 error_occurred 事件。
func Error(kind string, props map[string]any) {
	m := map[string]any{"kind": kind}
	maps.Copy(m, props)
	Track(EventError, m)
}

// AppStarted 启动时上报：首次启动额外发 app_installed，并 Identify 设备属性 + 发 app_started。
func AppStarted(appVersion, osName, osVersion, uiLang, channel string, isFirstLaunch bool) {
	// 启动日志：打印匿名统计开关状态（便于从日志确认是否在上报）。
	on, reasonKey := analyticsState()
	slog.Info(i18n.T("log.analytics_state"), "enabled", on, "reason", i18n.T(reasonKey))

	if isFirstLaunch && Enabled() {
		Track(EventAppInstalled, map[string]any{"app_version": appVersion, "channel": channel})
		// 标记已发送首装事件，避免重复（持久化到 settings）。
		mu.RLock()
		svc := ss
		mu.RUnlock()
		if svc != nil && svc.Get() != nil {
			svc.Get().AnalyticsInstalled = true
			_ = svc.Save()
		}
	}
	Identify(map[string]any{
		"app_version": appVersion,
		"os":          osName,
		"os_version":  osVersion,
		"ui_lang":     uiLang,
		"channel":     channel,
	})
	Track(EventAppStarted, map[string]any{
		"app_version": appVersion,
		"os":          osName,
		"os_version":  osVersion,
		"ui_lang":     uiLang,
		"channel":     channel,
	})
}

// SetEnabled 由前端开关调用：写回 settings 并持久化。
func SetEnabled(enabled bool) error {
	mu.Lock()
	svc := ss
	mu.Unlock()
	if svc == nil {
		return nil
	}
	svc.Get().AnalyticsEnabled = enabled
	if err := svc.Save(); err != nil {
		return err
	}
	if enabled {
		_ = ensureClient()
	} else {
		closeClient()
	}
	return nil
}

// Close 应用退出时调用：冲刷并关闭 client（停止连网）。
func Close() {
	closeClient()
}

func closeClient() {
	mu.Lock()
	defer mu.Unlock()
	if hasClient && client != nil {
		_ = client.Close()
		hasClient = false
		client = nil
	}
}

// LenBucket 把文本长度分桶，避免高基数属性。
func LenBucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n <= 20:
		return "1-20"
	case n <= 100:
		return "21-100"
	case n <= 500:
		return "101-500"
	case n <= 2000:
		return "501-2000"
	default:
		return "2000+"
	}
}
