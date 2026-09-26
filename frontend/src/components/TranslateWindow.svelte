<script lang="ts">
  import { onMount, tick } from 'svelte';
  import SpanText from './SpanText.svelte';
  import { t, langName, engineName } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { currentLang } from '../stores/ui';
  import { persisted, pinKey } from '../stores/persisted';
  import { rootStyleToStyle } from '../utils/style';
  import { onEvent, emitEvent } from '../runtime';
  import { Window, Clipboard } from '@wailsio/runtime';

  // Pin state persists to localStorage and survives reopening the window.
  // #39: pinned by default (modal semantics) — the working window stays above other apps until
  // explicitly hidden (hotkey toggle / red X / tray) and no longer "disappears" below them when
  // it loses focus. The 🧷 button keeps its meaning: the unpin/pin toggle.
  const PIN_MODAL_MIGRATION_KEY = 'kai:translate:pinnedModalDefault';
  const pinnedStore = persisted<boolean>(pinKey('translate'), true);
  // One-time migration: existing users were silently written false under the old default (false)
  // by subscribe, so merely flipping the constructed default does nothing for them — on seeing
  // "stored false and no migration flag" we reset to pinned and set the flag. Users who unpin
  // after the migration (flag already present) are always respected.
  if (typeof localStorage !== 'undefined' && !localStorage.getItem(PIN_MODAL_MIGRATION_KEY)) {
    if (localStorage.getItem(pinKey('translate')) === JSON.stringify(false)) {
      pinnedStore.set(true);
    }
    localStorage.setItem(PIN_MODAL_MIGRATION_KEY, '1');
  }
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

  // Auto-read clipboard and translate: when enabled, SaveConfig turns the copy hotkey
  // (execkeys.copy.enabled/fallback) off and records a snapshot, restored on disable, avoiding
  // double-triggering with the copy hotkey. The toggle state persists in settings.json
  // (auto_clipboard). When enabled, pressing the "input translation" hotkey makes the backend
  // read the system clipboard and translate directly — no more frontend polling.
  let autoClipboard = $state(false);

  async function applyAutoClipboard(next: boolean) {
    // Must save the full Settings, otherwise SaveConfig zeroes out the other fields.
    const cfg = (await GetConfig()) ?? ({} as any);
    if (next) {
      // Record the snapshot only if none exists yet (avoid double snapshots), and turn off both copy-hotkey switches.
      if (!cfg.copy_key_snapshot && cfg.execkeys?.copy) {
        cfg.copy_key_snapshot = { ...cfg.execkeys.copy };
      }
      if (cfg.execkeys?.copy) {
        cfg.execkeys.copy.enabled = false;
        cfg.execkeys.copy.fallback = false;
      }
      cfg.auto_clipboard = true;
    } else {
      // Restore the copy hotkey's original state and drop the snapshot.
      if (cfg.copy_key_snapshot && cfg.execkeys?.copy) {
        cfg.execkeys.copy.enabled = cfg.copy_key_snapshot.enabled;
        cfg.execkeys.copy.fallback = cfg.copy_key_snapshot.fallback;
      }
      cfg.copy_key_snapshot = null;
      cfg.auto_clipboard = false;
    }
    await SaveConfig(cfg as any);
    autoClipboard = next;
    // Broadcast to the settings page, disabling/restoring the copy-hotkey switches in real time.
    emitEvent(EventAutoClipboardChanged, next);
  }

  import {
    EventTranslateResult,
    EventInputFill,
    EventWindowClosing,
    EventEnginesChanged,
    EventAutoClipboardChanged,
  } from '../utils/events';
  import { WindowSettings, WindowTranslate } from '../constants/window';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import type {
    AllEngineItem,
    EngineListItem,
    NamedItem,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import { TRANSLATE_LANG, ALL_TRANSLATE_LANGS, type TranslateLang } from '../constants/lang';
  import { TranslateMulti } from '@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts';
  import { Learn as LearnLangVariant } from '@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts';
  import { learnFromSelection } from '../utils/langLearn.ts';
  import {
    GetEngines,
    GetAllEngines,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import {
    activeEngineFor,
    statusDots,
    anyPending,
    resetEdits,
    failureMessage,
    type DotState,
  } from '../utils/resultPane.ts';
  import { detectedSourceLabel } from '../utils/detectedLang.ts';
  import { flippedTargetLabel } from '../utils/flippedTarget.ts';
  import { isTargetDisabled } from '../utils/targetCapability.ts';
  import {
    GetLanguages,
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { ShowSettings } from '@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts';

  let input = $state('');
  let engines = $state<EngineListItem[]>([]);
  let languages = $state<NamedItem[]>([]);
  let fromLang = $state<TranslateLang>(TRANSLATE_LANG.Auto);
  let toLang = $state<TranslateLang>(TRANSLATE_LANG.EN);
  // Per-engine translation results, aggregated by engine name (multi-engine concurrency,
  // arriving one by one).
  let results = $state<Record<string, TranslateResult>>({});
  // The target the current results were requested with, captured when the request is sent (issue
  // #44). The backend may translate a result into a different target (the same-language guard
  // flips it to English) and reports that in the result's own `to`; comparing against this, not
  // the live select, keeps a later change of the select from making an untouched result look
  // flipped. Read-only feedback: nothing writes toLang from it.
  let requestedTo = $state<string>('');
  let loading = $state(false);

  // Two-pane layout (issue #10, locked decision: always side by side, no stacked fallback).
  // The divider splits the content row into the source pane (left) and the results pane (right);
  // the ratio persists to localStorage and survives reopening the window.
  // All clamping/conversion math lives in utils/paneLayout.ts (pure functions, covered by vitest).
  import { clampRatio, ratioFromPoint } from '../utils/paneLayout.ts';
  const dividerStore = persisted<number>('translate:divider', 0.5);
  let panesEl = $state<HTMLElement | null>(null);
  let leftRatio = $derived(clampRatio($dividerStore));

  // Divider drag: after mousedown, mousemove is tracked on window (dragging past the divider
  // keeps working); mouseup releases. Pointer horizontal position → row ratio → clamp → write
  // back to the persisted store.
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

  // Word-level span rendering (issue #11 groundwork): both panes' text renders as SpanText
  // (hover-highlighted spans), editing still goes through the textarea. Rule: pane text non-empty
  // and not in editing state → show the span layer; clicking the span layer switches back to the
  // textarea (word clicks themselves are deliberately inert; dictionary/alternatives are #18).
  // Empty text is always a textarea (placeholder visible, directly typeable).
  let editingSource = $state(true);
  let editingResult = $state(false);
  let sourceEl = $state<HTMLTextAreaElement | null>(null);
  let resultEl = $state<HTMLTextAreaElement | null>(null);

  function enterSourceEdit() {
    editingSource = true;
    tick().then(() => sourceEl?.focus());
  }

  function enterResultEdit() {
    editingResult = true;
    tick().then(() => resultEl?.focus());
  }

  // Translating marquee: animated ellipsis (. → .. → ... → .... cycling)
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

  // Last-used engine (persisted in localStorage, same mechanism as the persisted store's pinKey).
  // The highest-priority source when resolving the result pane's active engine (issue #9; written
  // by this window's engine dropdown, see handleEngineChange).
  const LAST_ENGINE_KEY = 'kai:translate:lastEngine';
  const lastUsedStore = persisted<string>(LAST_ENGINE_KEY, '');
  // settings' primary engine (default_engine): the middle layer of the resolution chain, read on mount.
  let defaultEngine = $state<string>('');
  // Engine list in the GetAllEngines shape (includes enabled/kind/supported): the primary-engine
  // resolution's enabled judgment needs it (GetEngines' EngineListItem has no enabled field).
  let allEngines = $state<AllEngineItem[]>([]);

  // Target-language options: engines like system translation don't support auto-detecting the
  // target language, so the target dropdown must exclude auto.
  const targetLanguages = $derived(languages.filter((l) => l.value !== TRANSLATE_LANG.Auto));

  // The engine the result pane is currently bound to (issue #9): activeEngineFor = #8's
  // resolvedPrimary derivation (last-used ?? primary(default_engine) ?? first-enabled) plus one
  // defensive fallback (falls back to the first engine when resolution is '' but enabled translate
  // engines still exist, guaranteeing the select never dangles). Recomputed whenever the engine
  // list / last-used / settings change; switching engines writes last-used (#8's setLastUsedEngine).
  const activeEngine = $derived(activeEngineFor(LAST_ENGINE_KEY, defaultEngine, allEngines));
  // One dot per enabled translate engine (state = the fan-out's real output: done/pending/failed, design §4).
  const dots = $derived(statusDots(allEngines, results, loading));
  // The active engine's current result (a failed engine is absent from results → null).
  const activeResult = $derived(activeEngine ? (results[activeEngine] ?? null) : null);
  // The text the active engine is showing / can show: manual edit ?? engine result ?? empty string.
  const activeDisplay = $derived(edited.get(activeEngine) ?? activeResult?.result ?? '');
  // The result-pane engine dropdown's display value: activeEngine already includes the
  // "'' → first enabled" defensive fallback, so it is never empty while activeEngines is non-empty
  // (the select never points at nothing).
  const firstEnabledName = $derived(activeEngines[0]?.value ?? '');
  const selectValue = $derived(activeEngine || firstEnabledName);
  // Auto-detect feedback (issue #11): when the source language is auto and the active engine has
  // returned a result, the language bar's auto option shows an "English (detected)"-style label;
  // it never overrides a pinned language.
  const detectedFrom = $derived(String(activeResult?.from ?? ''));
  function fromOptionLabel(value: string): string {
    const label = detectedSourceLabel(
      value,
      TRANSLATE_LANG.Auto,
      detectedFrom,
      langName,
      t('translate.detected'),
    );
    return label ?? langName(value);
  }
  // Same-language guard feedback (issue #44): when the backend flipped this result's target (the
  // detected source already was the requested target), the result card says which language it was
  // really translated into. Display only: the select keeps showing the requested target, and
  // nothing here assigns toLang / fromLang, persists, teaches or retranslates (both selects render
  // toLang, so writing it would make the flip sticky, and #13 owns the swap semantics).
  const flippedLabel = $derived(
    flippedTargetLabel(requestedTo, String(activeResult?.to ?? ''), langName),
  );
  // Result-pane manual edits (aggregated by engine name): discarded wholesale on engine switch /
  // retranslate / clearing input; a new engine always starts from its own stored result
  // (no per-engine edit memory, design §3).
  let edited = $state<Map<string, string>>(new Map());
  function setEdited(engine: string, value: string) {
    edited = new Map(edited).set(engine, value);
  }
  function handleEngineChange(ev: Event) {
    const name = (ev.currentTarget as HTMLSelectElement).value;
    setLastUsedEngine(name);
    // Engine switch: discard the previous engine's manual edit; the new engine starts from its own stored result.
    edited = resetEdits(edited, activeEngine, name, results);
    editingResult = false;
  }

  onMount(() => {
    // Event listeners must register first (so a result arriving during the awaited loads isn't lost).
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
      // Issue #69: opening Settings drops this window out of always-on-top so Settings is not
      // hidden behind a pinned window; when Settings closes, put the persisted pin back.
      if (name === WindowSettings) {
        Window.SetAlwaysOnTop($pinnedStore).catch((e) => console.error(t('log.restorePinFailed'), e));
        return;
      }
      // Global broadcast: only handle this window's (translate) closing, so closing another
      // window doesn't mistakenly clear the translation.
      if (name !== WindowTranslate) return;
      results = {};
      input = '';
      loading = false;
      edited = new Map();
    });
    // Broadcast after engines are added/removed or enabled/disabled in settings: re-fetch the
    // engine list so the translate window's result cards sync to the latest state (otherwise
    // enabled/disabled engines never refresh and the old list sticks).
    const offEngines = onEvent(EventEnginesChanged, () => {
      loadEngines();
    });
    // Initialization and first render: restore pin state, then wait for engines/languages/defaults to load.
    (async () => {
      // Restore the persisted pin state.
      try {
        await Window.SetAlwaysOnTop($pinnedStore);
      } catch (e) {
        console.error(t('log.restorePinFailed'), e);
      }
      // Must wait for engines/languages/defaults to load first (the result pane's dropdown and dots depend on them).
      await Promise.all([loadDefaults(), loadEngines(), loadLanguages()]);
      // Load the "auto-read clipboard" toggle + primary engine (both persisted in settings.json).
      // default_engine is the middle layer of the primary-engine resolution chain (last-used ?? primary ?? first-enabled).
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

  // Read default source/target languages from the settings file as initial values (fall back to auto/zh when unset).
  async function loadDefaults() {
    try {
      const cfg = await GetConfig();
      if (cfg?.default_from) fromLang = cfg.default_from as TranslateLang;
      if (cfg?.default_to) toLang = cfg.default_to as TranslateLang;
    } catch (e) {
      console.error(t('log.readDefaultLangFailed'), e);
    }
  }

  // Persist the current source/target languages to the settings file.
  async function persistLangs() {
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, default_from: fromLang, default_to: toLang });
    } catch (e) {
      console.error(t('log.persistLangPrefFailed'), e);
    }
  }

  // A language picked in either select (issue #53): teach the backend's variant-preference store,
  // then persist the choice as before. Picking es-MX once makes every later auto-detected Spanish
  // come back qualified as es-MX for this window session; the backend ignores bases and languages
  // without dialects, so every pick is passed through as it is.
  // Called from the selects' own onchange ONLY: swap() and loadDefaults() assign fromLang/toLang
  // directly and must never teach (an explicit choice is the only signal — "swap consumes, never
  // teaches"). Reads the picked value off the event so it doesn't depend on bind:value ordering.
  function onLangPicked(ev: Event) {
    learnLangVariant((ev.currentTarget as HTMLSelectElement).value as TranslateLang);
    persistLangs();
  }

  async function learnLangVariant(lang: TranslateLang) {
    await learnFromSelection(lang, LearnLangVariant, (e) =>
      console.error(t('log.learnLangVariantFailed'), e),
    );
  }

  const fallbackLanguages = $derived<NamedItem[]>(
    ALL_TRANSLATE_LANGS.map((c) => ({
      value: c,
      name: langName(c),
    })),
  );

  async function loadEngines() {
    // GetEngines: result-pane rendering (no enabled field; only translate/ocr + supported).
    // GetAllEngines: the enabled judgment for primary-engine resolution (includes enabled/kind/supported).
    // Both fetched in parallel to avoid a second round trip; on failure each falls back independently, never blocking the other.
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

  // Records the "last-used engine" to localStorage (last-used is the highest-priority source in
  // the primary-engine resolution chain).
  // Called by the result pane's engine selector (a later issue): when the user switches to an
  // engine, setLastUsedEngine(it) runs, and after reopening the window the first render binds
  // directly to it (if that engine is still enabled).
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
    // When the source language is "auto-detect" it cannot serve directly as the target language
    // (the target dropdown has no auto option). In that case land the source language on a
    // concrete language (zh) before swapping, so the swap always has a visible effect and toLang
    // never lands on auto.
    const from = fromLang === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.ZH : fromLang;
    const to = toLang === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.ZH : toLang;
    fromLang = to;
    toLang = from;
    persistLangs();
  }

  async function doTranslate() {
    if (!input.trim() || activeEngines.length === 0) return;
    loading = true;
    requestedTo = toLang;
    results = {};
    // A new fan-out round starts blank: the previous batch's edits are meaningless for the new
    // round and are discarded with it.
    edited = new Map();
    editingResult = false;
    try {
      // Multi-engine concurrency is handled in parallel by the backend across enabled engines, independent of any single engine;
      // the bindings-generated TranslateRequest.engine is required, so pass an empty string to satisfy the type (the backend ignores it).
      await TranslateMulti({
        text: input,
        from: fromLang as TranslateLang,
        to: toLang as TranslateLang,
        engine: '',
      });
      // Results arrive asynchronously one by one via EventTranslateResult; the template clears
      // loading when the first result lands.
    } catch (e) {
      console.error(t('log.translateRequestFailed'), e);
    } finally {
      // Fallback (relaxed, design §4): at 15 s, if the fan-out is still in progress (any enabled
      // translate engine still pending), clear loading anyway; engines whose siblings already
      // flipped loading to false don't get re-flipped here. The old "zero results" predicate left
      // a failed engine's dot stuck on pending forever — relaxed to "any pending", case 2's sole
      // failed engine converges at this fallback point, and the same moment loading clears, its
      // dot flips from pending to failed.
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
    editingResult = false;
  }
</script>

<div class="u-surface flex h-screen flex-col" style={rootStyleToStyle($rootStyle)}>
  <main class="flex h-full min-h-0 flex-col gap-4 overflow-hidden p-4">
    <!-- Toolbar row: spans both panes (from/swap/to apply to the whole translation; DeepL-style
         layout). Three columns with equal 1fr sides keep the from/swap/to group centered while
         the Settings gear (issue #69) sits alone in the right column, at the row's right end. -->
    <div class="grid grid-cols-[1fr_auto_1fr] items-center gap-2">
      <div class="col-start-2 flex items-center justify-center gap-2">
        <select
          class="u-field u-select u-lang-select px-3 py-2 text-sm"
          bind:value={fromLang}
          onchange={onLangPicked}
          aria-label={t('translate.from')}
        >
          {#each languages as l}
            <option value={l.value}>{fromOptionLabel(l.value)}</option>
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
          onchange={onLangPicked}
          aria-label={t('translate.to')}
        >
          {#each targetLanguages as l}
            <!-- issue #52: a target no enabled engine can translate into is shown disabled;
                 the capability comes from the backend (allEngines[].target_languages), never a
                 frontend map. The source select above is never gated (every engine accepts every
                 source). -->
            <option value={l.value} disabled={isTargetDisabled(allEngines, l.value)}>
              {langName(l.value)}
            </option>
          {/each}
        </select>
      </div>

      <!-- Settings gear (issue #69): app-level configuration, so it sits at the toolbar's right end,
           outside both cards. Opens the Settings window through the existing WindowWrapper
           binding (ShowSettings, no new Go API); this window stays open. Not a toggle, so it never
           gets the --active styling. Glyph: Lucide "settings" (ISC), inline like the swap
           button's. Label and tooltip reuse the titlebar.settings key. -->
      <div class="col-start-3 flex justify-end">
        <button
          class="u-icon-btn u-no-drag"
          onclick={() => ShowSettings()}
          aria-label={t('titlebar.settings')}
          title={t('titlebar.settings')}
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
            <path
              d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"
            />
            <circle cx="12" cy="12" r="3" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Two-pane row (issue #10, locked decision: always side by side, no stacked fallback):
         left = source text, right = results; the divider between panes is draggable (ratio
         persists to localStorage, math in utils/paneLayout.ts) -->
    <div bind:this={panesEl} class="flex min-h-0 flex-1 gap-3">
      <!-- Left pane: source text (width = persisted ratio, changed by divider drag; the textarea fills the remaining height) -->
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
        {#if input === '' || editingSource}
          <textarea
            bind:this={sourceEl}
            class="min-h-0 flex-1 resize-none bg-transparent p-4 text-base leading-relaxed outline-none"
            bind:value={input}
            onfocus={() => (editingSource = true)}
            onblur={() => (editingSource = false)}
            placeholder={t('translate.placeholder')}></textarea>
        {:else}
          <!-- Non-editing state: word-level span rendering (hover highlight, no click behavior — #18) -->
          <div
            class="min-h-0 flex-1 cursor-text overflow-y-auto p-4 text-base leading-relaxed"
            onclick={enterSourceEdit}
          >
            <SpanText text={input} />
          </div>
        {/if}
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

      <!-- Divider: dragging changes the left pane's ratio (see startDividerDrag / paneLayout.ts) -->
      <div
        role="separator"
        aria-orientation="vertical"
        class="w-1.5 shrink-0 cursor-col-resize rounded-full bg-[var(--app-muted)] opacity-40 transition-opacity hover:opacity-100 u-no-drag"
        onmousedown={startDividerDrag}
      ></div>

      <!-- Right pane: results (issue #9's active-engine single card + engine dropdown + status dots, migrated as-is) -->
      <section class="u-card u-card--panel flex min-w-0 flex-1 flex-col overflow-hidden">
        <div class="u-border-b flex items-center justify-between px-3 py-2">
          <span class="u-label">{t('translate.result')}</span>
          <div class="flex items-center gap-2">
            {#if activeEngines.length > 0}
              <!-- Result-pane engine dropdown (design §2): controlled display value = the active engine
                 (last-used/primary derivation, see activeEngineFor); onchange writes last-used
                 (#8's setLastUsedEngine) and resets edits. Disabled engines (just toggled in
                 settings, before EventEnginesChanged lands) are listed as disabled. -->
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
              <!-- One status dot per engine (design §4): done/pending/failed derived purely from the
                 fan-out's real output; the active engine's dot gets an accent ring so the dropdown's
                 selection is visible at a glance. -->
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
            <!-- Copy button: copies only the active engine's currently displayed text (including
                 manual edits), no longer concatenating all engines (design §6). -->
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
            <!-- The active engine is still in flight (no result yet): loading placeholder (kai-dots + kai-loading-bar). -->
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
            <!-- The active engine has a (non-empty) result: editable single card (design §5). Edits
               write back to edited[activeEngine]; displayed text = edited ?? result. On engine switch
               edited is discarded wholesale and the new engine starts from its own result. -->
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
              {#if flippedLabel}
                <!-- Same-language guard (issue #44): this result went to a different target than
                   the one selected; say which. Display only, never changes the select. -->
                <p class="u-muted mb-2 text-xs" data-testid="flipped-target">
                  {t('translate.flippedTo', { lang: flippedLabel })}
                </p>
              {/if}
              {#if editingResult || activeDisplay === ''}
                <textarea
                  bind:this={resultEl}
                  class="min-h-[120px] resize-none bg-transparent p-4 text-base leading-relaxed outline-none"
                  value={activeDisplay}
                  onchange={(ev) => setEdited(activeEngine, ev.currentTarget.value)}
                  onblur={() => (editingResult = false)}
                  placeholder={t('translate.noResult')}></textarea>
              {:else}
                <!-- Non-editing state: word-level span rendering (hover highlight); click to edit (#9's editable semantics kept) -->
                <div
                  class="min-h-[120px] cursor-text text-base leading-relaxed"
                  onclick={enterResultEdit}
                >
                  <SpanText text={activeDisplay} />
                </div>
              {/if}
            </div>
          {:else}
            <!-- The active engine failed (absent from results with loading already cleared) or returned
               an empty result: failed state (design §5). No retry, no retry button — retrying means the
               user presses the translate button again (re-running the whole fan-out). -->
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
