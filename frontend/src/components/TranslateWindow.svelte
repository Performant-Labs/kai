<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { currentLang } from '../stores/ui';
  import { persisted, pinKey } from '../stores/persisted';
  import { rootStyleToStyle } from '../utils/style';
  import { onEvent, emitEvent } from '../runtime';
  import { Window, Clipboard } from '@wailsio/runtime';

  // 置顶状态持久化到 localStorage，重开窗口后保留。
  const pinnedStore = persisted<boolean>(pinKey('translate'), false);
  let pinned = $derived($pinnedStore);
  async function togglePin() {
    const next = !pinned;
    pinnedStore.set(next);
    try {
      await Window.SetAlwaysOnTop(next);
    } catch (e) {
      console.error('toggle pin failed', e);
    }
  }

  // 自动读取剪贴板并翻译：开启后经 SaveConfig 把复制键（execkeys.copy.enabled/fallback）
  // 置 false 并记录快照，关闭时恢复，避免与复制键双重触发。开关状态持久化在 settings.json
  // （auto_clipboard）。开启后按下「输入翻译」快捷键即由后端直接读取系统剪贴板并翻译，
  // 不再由前端轮询。
  let autoClipboard = $state(false);

  async function applyAutoClipboard(next: boolean) {
    // 必须用完整 Settings 保存，否则 SaveConfig 会把其它字段清零。
    const cfg = (await GetConfig()) ?? ({} as any);
    if (next) {
      // 仅在尚无快照时记录（防重复快照），并关闭复制键两个开关。
      if (!cfg.copy_key_snapshot && cfg.execkeys?.copy) {
        cfg.copy_key_snapshot = { ...cfg.execkeys.copy };
      }
      if (cfg.execkeys?.copy) {
        cfg.execkeys.copy.enabled = false;
        cfg.execkeys.copy.fallback = false;
      }
      cfg.auto_clipboard = true;
    } else {
      // 恢复复制键原状态并清掉快照。
      if (cfg.copy_key_snapshot && cfg.execkeys?.copy) {
        cfg.execkeys.copy.enabled = cfg.copy_key_snapshot.enabled;
        cfg.execkeys.copy.fallback = cfg.copy_key_snapshot.fallback;
      }
      cfg.copy_key_snapshot = null;
      cfg.auto_clipboard = false;
    }
    await SaveConfig(cfg as any);
    autoClipboard = next;
    // 广播给设置页，实时禁用/恢复复制键开关。
    emitEvent(EventAutoClipboardChanged, next);
  }

  import {
    EventTranslateResult,
    EventInputFill,
    EventWindowClosing,
    EventEnginesChanged,
    EventAutoClipboardChanged,
  } from '../utils/events';
  import { WindowTranslate } from '../constants/window';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import type {
    AllEngineItem,
    EngineListItem,
    NamedItem,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import { TRANSLATE_LANG, ALL_TRANSLATE_LANGS, type TranslateLang } from '../constants/lang';
  import { TranslateMulti } from '@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts';
  import {
    GetEngines,
    GetAllEngines,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import {
    activeEngineFor,
    statusDots,
    anyPending,
    resetEdits,
    type DotState,
  } from '../utils/resultPane.ts';
  import {
    GetLanguages,
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';

  let input = $state('');
  let engines = $state<EngineListItem[]>([]);
  let languages = $state<NamedItem[]>([]);
  let fromLang = $state<TranslateLang>(TRANSLATE_LANG.Auto);
  let toLang = $state<TranslateLang>(TRANSLATE_LANG.EN);
  // 各引擎翻译结果，按引擎名聚合（多引擎并发，逐个到达）。
  let results = $state<Record<string, TranslateResult>>({});
  let loading = $state(false);

  // 两栏布局（issue #10，锁定决策：永远左右并排，无堆叠回退）。分隔条把内容行切成
  // 源文本栏（左）与结果栏（右），占比持久化到 localStorage，重开窗口后保留。
  // 夹逼/换算逻辑全部在 utils/paneLayout.ts（纯函数，vitest 覆盖）。
  import { clampRatio, ratioFromPoint } from '../utils/paneLayout.ts';
  const dividerStore = persisted<number>('translate:divider', 0.5);
  let panesEl = $state<HTMLElement | null>(null);
  let leftRatio = $derived(clampRatio($dividerStore));

  // 分隔条拖拽：mousedown 后在 window 上跟踪 mousemove（拖出分隔条也能继续拖），
  // mouseup 解除。指针水平位置 → 行内占比 → 夹逼 → 写回持久化 store。
  function startDividerDrag(ev: MouseEvent) {
    ev.preventDefault();
    const row = panesEl;
    if (!row) return;
    const move = (e: MouseEvent) => {
      dividerStore.set(
        ratioFromPoint(e.clientX, row.getBoundingClientRect().left, row.clientWidth),
      );
    };
    const up = () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }

  // 翻译中走马灯：动态省略号（. → .. → ... → .... 循环）
  let dotCount = $state(0);
  $effect(() => {
    if (!loading) {
      dotCount = 0;
      return;
    }
    const timer = setInterval(() => {
      dotCount = (dotCount + 1) % 4;
    }, 400);
    return () => clearInterval(timer);
  });

  const curLang = $derived(currentLang());

  const activeEngines = $derived(engines.filter((e) => e.kind === 'translate'));

  // 上次使用的引擎（localStorage 持久化，见 persisted store 的 pinKey 同款机制）。
  // 结果区活动引擎解析的最优先来源（issue #9 由本窗口的引擎下拉框写入，见 handleEngineChange）。
  const LAST_ENGINE_KEY = 'kai:translate:lastEngine';
  const lastUsedStore = persisted<string>(LAST_ENGINE_KEY, '');
  // settings 的主引擎（default_engine）：解析链的中间层，onMount 时读取。
  let defaultEngine = $state<string>('');
  // GetAllEngines 形态的引擎列表（含 enabled/kind/supported）：主引擎解析的 enabled 判定
  // 需要它（GetEngines 的 EngineListItem 没有 enabled 字段）。
  let allEngines = $state<AllEngineItem[]>([]);

  // 目标语言选项：系统翻译等引擎不支持自动检测目标语言，目标语言下拉框必须排除 auto。
  const targetLanguages = $derived(languages.filter((l) => l.value !== TRANSLATE_LANG.Auto));

  // 结果区当前绑定的引擎（issue #9）：activeEngineFor = #8 的 resolvedPrimary 推导
  // （last-used ?? primary(default_engine) ?? first-enabled）+ 一层防御性回退（解析为 '' 但
  // 仍有 enabled 翻译引擎时回退到第一个，保证 select 不悬空）。引擎列表 / last-used /
  // settings 任一变化即重算；切引擎即写入 last-used（#8 的 setLastUsedEngine）。
  const activeEngine = $derived(activeEngineFor(LAST_ENGINE_KEY, defaultEngine, allEngines));
  // 每个 enabled 翻译引擎一个 dot（状态 = fan-out 真实输出：done/pending/failed，设计 §4）。
  const dots = $derived(statusDots(allEngines, results, loading));
  // 活动引擎的当前结果（失败的引擎在 results 里缺席 → null）。
  const activeResult = $derived(activeEngine ? (results[activeEngine] ?? null) : null);
  // 活动引擎正在显示/可显示的文本：手工编辑 ?? 引擎结果 ?? 空串。
  const activeDisplay = $derived(edited.get(activeEngine) ?? activeResult?.result ?? '');
  // 结果区引擎下拉框的显示值：activeEngine 已含「'' → 第一个 enabled」防御回退，
  // 故 activeEngines 非空时必非空（select 永不指向 nothing）。
  const firstEnabledName = $derived(activeEngines[0]?.value ?? '');
  const selectValue = $derived(activeEngine || firstEnabledName);
  // 结果区手工编辑（按引擎名聚合）：切换引擎 / 重新翻译 / 清空输入时整体丢弃，
  // 新引擎一律从它自己的存储结果起步（无 per-engine 编辑记忆，设计 §3）。
  let edited = $state<Map<string, string>>(new Map());
  function setEdited(engine: string, value: string) {
    edited = new Map(edited).set(engine, value);
  }
  function handleEngineChange(ev: Event) {
    const name = (ev.currentTarget as HTMLSelectElement).value;
    setLastUsedEngine(name);
    // 切换引擎：丢弃上一个引擎的手工编辑，新引擎从它自己的存储结果起步。
    edited = resetEdits(edited, activeEngine, name, results);
  }

  onMount(() => {
    // 事件监听需先注册（await 加载期间若有翻译结果到达也不丢失）。
    const offResult = onEvent(EventTranslateResult, (payload: TranslateResult) => {
      if (payload && payload.engine) {
        results = { ...results, [payload.engine]: payload };
        loading = false;
      }
    });
    const offInputFill = onEvent(EventInputFill, (text: string) => {
      if (!text) return;
      input = text;
      doTranslate();
    });
    const offClosing = onEvent(EventWindowClosing, (name: string) => {
      // 全局广播：只处理本窗口（translate）的关闭，避免关闭别的窗口误清空翻译。
      if (name !== WindowTranslate) return;
      results = {};
      input = '';
      loading = false;
      edited = new Map();
    });
    // 设置里增删/启停引擎后广播：重新拉取引擎列表，使翻译窗口结果卡片同步最新状态
    // （否则开启/关闭的引擎不会刷新，仍是旧列表）。
    const offEngines = onEvent(EventEnginesChanged, () => {
      loadEngines();
    });
    // 初始化与首屏：恢复置顶状态，等引擎/语言/默认值加载完成。
    (async () => {
      // 恢复持久化的置顶状态。
      try {
        await Window.SetAlwaysOnTop($pinnedStore);
      } catch (e) {
        console.error(t('log.restorePinFailed'), e);
      }
      // 必须先等引擎/语言/默认值加载完（结果区下拉框与 dots 依赖它们）。
      await Promise.all([loadDefaults(), loadEngines(), loadLanguages()]);
      // 载入「自动读取剪贴板翻译」开关 + 主引擎（均持久化在 settings.json）。
      // default_engine 是主引擎解析链的中间层（last-used ?? primary ?? first-enabled）。
      try {
        const cfg = await GetConfig();
        if (cfg?.auto_clipboard) {
          autoClipboard = true;
        }
        if (cfg?.default_engine) {
          defaultEngine = cfg.default_engine;
        }
      } catch (e) {
        console.error(t('log.autoClipboardLoadFailed'), e);
      }
    })();
    return () => {
      offResult();
      offInputFill();
      offClosing();
      offEngines();
    };
  });

  // 从设置文件读取默认源/目标语言作为初始值（未配置回退 auto/zh）。
  async function loadDefaults() {
    try {
      const cfg = await GetConfig();
      if (cfg?.default_from) fromLang = cfg.default_from as TranslateLang;
      if (cfg?.default_to) toLang = cfg.default_to as TranslateLang;
    } catch (e) {
      console.error(t('log.readDefaultLangFailed'), e);
    }
  }

  // 持久化当前源/目标语言到设置文件。
  async function persistLangs() {
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, default_from: fromLang, default_to: toLang });
    } catch (e) {
      console.error(t('log.persistLangPrefFailed'), e);
    }
  }

  const fallbackLanguages = $derived<NamedItem[]>(
    ALL_TRANSLATE_LANGS.map((c) => ({
      value: c,
      name: langName(c),
    })),
  );

  async function loadEngines() {
    // GetEngines：结果区渲染（无 enabled 字段，只区分 translate/ocr + supported）。
    // GetAllEngines：主引擎解析的 enabled 判定（含 enabled/kind/supported）。
    // 二者并行拉取，避免二次往返；任一失败各自回退，互不阻塞。
    try {
      const [list, all] = await Promise.all([GetEngines(), GetAllEngines()]);
      engines = list ?? [];
      allEngines = all ?? [];
    } catch (e) {
      console.error(t('log.loadEngineListFailed'), e);
      engines = [];
      allEngines = [];
    }
  }

  // 记录「上次使用的引擎」到 localStorage（last-used 是主引擎解析链的最优先来源）。
  // 由下一 issue 的结果区引擎选择器调用：用户切到某引擎即 setLastUsedEngine(它)，
  // 之后重开窗口首屏直接绑定它（若该引擎仍 enabled）。
  function setLastUsedEngine(name: string) {
    lastUsedStore.set(name);
  }

  async function loadLanguages() {
    try {
      const list = await GetLanguages(curLang);
      languages = list?.length ? list : fallbackLanguages;
    } catch (e) {
      console.error(t('log.loadLangListFailed'), e);
      languages = fallbackLanguages;
    }
  }

  function swap() {
    // 复制键/源语言为「自动检测」时无法直接作为目标语言（目标下拉框无 auto 选项）。
    // 此时把源语言落到一个具体语言（zh）再交换，保证交换永远有可见效果，且 toLang 不会落到 auto。
    const from = fromLang === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.ZH : fromLang;
    const to = toLang === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.ZH : toLang;
    fromLang = to;
    toLang = from;
    persistLangs();
  }

  async function doTranslate() {
    if (!input.trim() || activeEngines.length === 0) return;
    loading = true;
    results = {};
    // 新一轮 fan-out 从空白开始：上一批的编辑结果对新一轮无意义，一并丢弃。
    edited = new Map();
    try {
      // 多引擎并发由后端按已开启引擎并行，不依赖单个 engine；
      // bindings 生成的 TranslateRequest.engine 为必填，传空串以满足类型（后端忽略）。
      await TranslateMulti({
        text: input,
        from: fromLang as TranslateLang,
        to: toLang as TranslateLang,
        engine: '',
      });
      // 结果通过 EventTranslateResult 逐个异步到达，loading 在收到首个结果时由模板判断解除。
    } catch (e) {
      console.error(t('log.translateRequestFailed'), e);
    } finally {
      // 兜底（放宽，设计 §4）：15 s 时若 fan-out 仍在进行（任一 enabled 翻译引擎仍 pending）
      // 就解除 loading；兄弟引擎各自到达已把 loading 翻 false 的，此处不再回退。原「零结果」
      // 谓词会让失败引擎的 dot 永远停在 pending——放宽到「任一 pending」后，case 2 的唯一失败
      // 引擎由本兜底点收敛，loading 解除的同一刻其 dot 由 pending 翻转为 failed。
      setTimeout(() => {
        if (anyPending(allEngines, results, loading)) loading = false;
      }, 15000);
    }
  }

  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  function showToast(msg: string) {
    toast = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = ''), 1600);
  }

  async function copy(text: string) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      showToast(t('common.copied'));
    } catch (e) {
      console.error('copy failed', e);
    }
  }

  function clearInput() {
    input = '';
    results = {};
    edited = new Map();
  }
</script>

<div class="u-surface flex h-screen flex-col" style={rootStyleToStyle($rootStyle)}>
  <main class="flex h-full min-h-0 flex-col gap-4 overflow-hidden p-4">
    <!-- 语言控制条：横贯两栏之上（from/swap/to 作用于整次翻译，DeepL 同款布局） -->
    <div class="flex items-center justify-center gap-2">
      <select
        class="u-field u-select u-lang-select px-3 py-2 text-sm"
        bind:value={fromLang}
        onchange={persistLangs}
        aria-label={t('translate.from')}
      >
        {#each languages as l}
          <option value={l.value}>{langName(l.value)}</option>
        {/each}
      </select>

      <button
        class="u-icon-btn u-no-drag"
        onclick={swap}
        aria-label={t('translate.swap')}
        title={t('translate.swap')}
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M7 10h14l-4-4" />
          <path d="M17 14H3l4 4" />
        </svg>
      </button>

      <select
        class="u-field u-select u-lang-select px-3 py-2 text-sm"
        bind:value={toLang}
        onchange={persistLangs}
        aria-label={t('translate.to')}
      >
        {#each targetLanguages as l}
          <option value={l.value}>{langName(l.value)}</option>
        {/each}
      </select>
    </div>

    <!-- 两栏行（issue #10，锁定决策：永远左右并排，无堆叠回退）：左 = 源文本，右 = 结果，
         中缝分隔条可拖拽（占比持久化到 localStorage，数学在 utils/paneLayout.ts） -->
    <div bind:this={panesEl} class="flex min-h-0 flex-1 gap-3">
      <!-- 左栏：源文本（宽度 = 持久化占比，分隔条拖拽改变；textarea 撑满余高） -->
      <section
        class="u-card u-card--panel flex min-w-0 flex-col overflow-hidden"
        style="width: {leftRatio * 100}%"
      >
        <div class="u-border-b flex items-center justify-between px-3 py-2">
          <span class="u-label">{t('translate.from')}</span>
          <div class="flex items-center gap-2">
            {#if activeEngines.length === 0}
              <span class="u-muted text-xs">{t('translate.noActiveEngine')}</span>
            {:else}
              <span class="u-muted text-xs"
                >{activeEngines.length} · {t('translate.multiEngineHint')}</span
              >
            {/if}
            <button
              class="u-icon-btn u-icon-btn--sm u-no-drag"
              class:u-icon-btn--active={pinned}
              onclick={togglePin}
              aria-label={pinned ? t('translate.unpin') : t('translate.pin')}
              title={pinned ? t('translate.unpin') : t('translate.pin')}
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M12 17v5" />
                <path
                  d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"
                />
              </svg>
            </button>
            <button
              class="u-icon-btn u-icon-btn--sm u-no-drag"
              class:u-icon-btn--active={autoClipboard}
              onclick={() => applyAutoClipboard(!autoClipboard)}
              aria-label={t('translate.autoClipboard')}
              title={t('translate.autoClipboard')}
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <rect x="8" y="2" width="8" height="4" rx="1" ry="1"></rect>
                <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"
                ></path>
              </svg>
            </button>
          </div>
        </div>
        <textarea
          class="min-h-0 flex-1 resize-none bg-transparent p-4 text-base leading-relaxed outline-none"
          bind:value={input}
          placeholder={t('translate.placeholder')}></textarea>
        <div class="u-border-t flex items-center justify-between px-3 py-2">
          <button class="u-btn u-btn--ghost u-no-drag px-3 py-1.5 text-sm" onclick={clearInput}>
            {t('translate.clearInput')}
          </button>
          <div class="flex items-center gap-2">
            <button
              class="u-icon-btn u-no-drag"
              onclick={() => copy(input)}
              aria-label={t('common.copy')}
              title={t('common.copy')}
              disabled={!input}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
              </svg>
            </button>
            <button
              class="u-btn u-btn--primary u-no-drag px-5 py-1.5 text-sm"
              onclick={doTranslate}
              disabled={loading || !input.trim()}
            >
              {loading ? t('common.loading') : t('translate.button')}
            </button>
          </div>
        </div>
      </section>

      <!-- 分隔条：拖拽改变左栏占比（见 startDividerDrag / paneLayout.ts） -->
      <div
        role="separator"
        aria-orientation="vertical"
        class="w-1.5 shrink-0 cursor-col-resize rounded-full bg-[var(--app-muted)] opacity-40 transition-opacity hover:opacity-100 u-no-drag"
        onmousedown={startDividerDrag}
      ></div>

      <!-- 右栏：结果（issue #9 的活动引擎单卡 + 引擎下拉框 + 状态 dots 原样迁入） -->
      <section class="u-card u-card--panel flex min-w-0 flex-1 flex-col overflow-hidden">
        <div class="u-border-b flex items-center justify-between px-3 py-2">
          <span class="u-label">{t('translate.result')}</span>
          <div class="flex items-center gap-2">
            {#if activeEngines.length > 0}
              <!-- 结果区引擎下拉框（设计 §2）：受控显示值 = 活动引擎（last-used/primary 推导，
                 见 activeEngineFor）；onchange 写 last-used（#8 的 setLastUsedEngine）并重置
                 编辑。禁用的引擎（settings 刚切换、EventEnginesChanged 尚未落地）列为 disabled。 -->
              <select
                class="u-field u-select u-engine-select px-3 py-2 text-sm"
                value={selectValue}
                onchange={handleEngineChange}
                aria-label={t('translate.engineActive')}
                title={t('translate.engineActive')}
              >
                {#each activeEngines as e}
                  <option value={e.value} disabled={!e.enabled}>
                    {engineName(e.value)}{!e.enabled ? t('translate.engineDisabled') : ''}
                  </option>
                {/each}
              </select>
              <!-- 每引擎一个状态 dot（设计 §4）：done/pending/failed 纯由 fan-out 真实输出派生；
                 活动引擎的 dot 加 accent 环，让下拉框的选择一眼可见。 -->
              <div class="flex items-center gap-1">
                {#each activeEngines as e (e.value)}
                  {@const st = dots[e.value] as DotState}
                  <span
                    class="h-2 w-2 rounded-full"
                    class:bg-[var(--app-accent)]={st === 'done'}
                    class:bg-[var(--app-muted)]={st === 'pending'}
                    class:bg-[var(--app-danger)]={st === 'failed'}
                    class:ring-2={e.value === activeEngine}
                    class:ring-[var(--app-accent)]={e.value === activeEngine}
                    title={engineName(e.value) +
                      (st === 'done'
                        ? ' · ' + t('translate.engineDone')
                        : st === 'pending'
                          ? ' · ' + t('translate.enginePending')
                          : ' · ' + t('translate.engineFailed'))}
                    aria-label={engineName(e.value) +
                      (st === 'done'
                        ? ' · ' + t('translate.engineDone')
                        : st === 'pending'
                          ? ' · ' + t('translate.enginePending')
                          : ' · ' + t('translate.engineFailed'))}
                  ></span>
                {/each}
              </div>
            {/if}
            <!-- 复制按钮：只复制活动引擎当前显示文本（含手工编辑），不再拼接所有引擎（设计 §6）。 -->
            {#if activeResult?.result || edited.has(activeEngine)}
              <button
                class="u-icon-btn u-no-drag"
                onclick={() => copy(activeDisplay)}
                aria-label={t('translate.copy')}
                title={t('translate.copy')}
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
              </button>
            {/if}
          </div>
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto p-4">
          {#if activeEngines.length === 0}
            <div class="flex h-full flex-col items-center justify-center gap-2 text-center">
              <svg
                class="u-muted"
                width="40"
                height="40"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
              </svg>
              <span class="u-muted text-sm">{t('translate.noActiveEngine')}</span>
            </div>
          {:else if loading && !results[activeEngine]}
            <!-- 活动引擎尚在飞行（尚无结果）：loading 占位（kai-dots + kai-loading-bar）。 -->
            <div class="u-result-card">
              <div class="mb-2 flex items-center gap-2">
                <span
                  class="rounded-full bg-[var(--app-accent)] px-2 py-0.5 text-xs font-medium text-[var(--app-accent-fg)]"
                >
                  {engineName(activeEngine)}
                </span>
              </div>
              <div class="flex flex-col gap-2">
                <p class="u-muted text-base leading-relaxed">
                  {t('common.loading')}<span class="kai-dots">{'.'.repeat(dotCount)}</span>
                </p>
                <div class="kai-loading-bar" aria-hidden="true"></div>
              </div>
            </div>
          {:else if activeResult?.result}
            <!-- 活动引擎已有（非空）结果：可编辑单卡（设计 §5）。编辑写回 edited[activeEngine]，
               显示文本 = edited ?? result；切换引擎时 edited 整体丢弃，新引擎从自身结果起步。 -->
            <div class="u-result-card">
              <div class="mb-2 flex items-center gap-2">
                <span
                  class="rounded-full bg-[var(--app-accent)] px-2 py-0.5 text-xs font-medium text-[var(--app-accent-fg)]"
                >
                  {engineName(activeEngine)}
                </span>
                {#if activeResult.phonetic}
                  <span class="u-muted text-xs">{activeResult.phonetic}</span>
                {/if}
              </div>
              <textarea
                class="min-h-[120px] resize-none bg-transparent p-4 text-base leading-relaxed outline-none"
                value={activeDisplay}
                onchange={(ev) => setEdited(activeEngine, ev.currentTarget.value)}
                placeholder={t('translate.noResult')}></textarea>
            </div>
          {:else}
            <!-- 活动引擎失败（缺席于 results 且 loading 已解除）或引擎返回空结果：失败态（设计 §5）。
               不重试、无重试按钮——重试即用户重按翻译按钮（重跑整个 fan-out）。 -->
            <div class="flex h-full flex-col items-center justify-center gap-2 text-center">
              <span class="text-sm" style="color: var(--app-danger)">{t('translate.failed')}</span>
            </div>
          {/if}
        </div>
      </section>
    </div>
  </main>

  {#if toast}
    <div class="u-toast">{toast}</div>
  {/if}
</div>
