<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../i18n';
  import { onEvent, emitEvent, Window } from '../runtime';
  import { Clipboard } from '@wailsio/runtime';
  import {
    EventScreenshotOCR,
    EventScreenshotRecapture,
    EventScreenshotRetranslate,
    EventWindowClosing,
    EventEnginesChanged,
    ScreenshotSessionScreenshot,
  } from '../utils/events';
  import { WindowScreenshot } from '../constants/window';
  import TranslateCard from './TranslateCard.svelte';
  import {
    TRANSLATE_LANG,
    ALL_TRANSLATE_LANGS,
    TARGET_TRANSLATE_LANGS,
    type TranslateLang,
  } from '../constants/lang';
  import type {
    ScreenshotResult,
    TranslateResult,
  } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import type { AllEngineItem } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import type { ScreenshotRetranslatePayload } from '../utils/events';
  import { GetConfig } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { GetAllEngines } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import { Learn as LearnLangVariant } from '@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts';
  import { learnFromSelection } from '../utils/langLearn.ts';
  import { isTargetDisabled } from '../utils/targetCapability.ts';
  import { persisted, pinKey } from '../stores/persisted';

  // Pin state persists to localStorage, a memory independent of the translate window's (pinKey('translate')).
  const pinnedStore = persisted<boolean>(pinKey('screenshot'), false);
  let pinned = $derived($pinnedStore);
  async function togglePin() {
    const next = !pinned;
    pinnedStore.set(next);
    // Only call SetAlwaysOnTop(true) when the user actively pins. Note: never call
    // SetAlwaysOnTop(false) when next=false — that would push the window level (floating, set at
    // creation) back down to NSNormal, reintroducing occlusion. The window level (floating) is
    // guaranteed by main.go's AlwaysOnTop:true; pinning only remembers the user's preference.
    if (next) {
      try {
        await Window.SetAlwaysOnTop(true);
      } catch (e) {
        console.error(t('log.screenshotSetPinFailed'), e);
      }
    }
  }

  let result = $state<ScreenshotResult | null>(null);
  let imgEl: HTMLImageElement | undefined = $state();

  // The screenshot-translation language bar: shows source/target languages, directly user-selectable.
  // After a language change the frontend emits EventScreenshotRetranslate with a debounce; the backend
  // reuses the last OCR'd source text and retranslates directly (skipping screenshot/OCR).
  let fromLang = $state<TranslateLang>(TRANSLATE_LANG.Auto);
  let toLang = $state<TranslateLang>(TRANSLATE_LANG.EN);

  // issue #52: per-engine target capability (backend-owned: AllEngineItem.target_languages),
  // used to disable the target options no enabled engine can translate into. Reloaded when
  // engines are added / removed / toggled in the settings while this window lives on hidden.
  let allEngines = $state<AllEngineItem[]>([]);
  async function loadCapability() {
    try {
      allEngines = (await GetAllEngines()) ?? [];
    } catch (e) {
      // Fail open: without capability data nothing is disabled (see isTargetDisabled).
      console.error(t('log.loadEngineListFailed'), e);
      allEngines = [];
    }
  }

  // Read default source/target languages from settings as the language bar's initial display values.
  async function loadDefaults() {
    try {
      const cfg = await GetConfig();
      if (cfg?.default_from) fromLang = cfg.default_from as TranslateLang;
      if (cfg?.default_to) toLang = cfg.default_to as TranslateLang;
    } catch (e) {
      console.error(t('log.readDefaultLangFailed'), e);
    }
  }

  // retranslateTimer debounce: trigger the retranslate 300ms after a language change, avoiding
  // frequent requests during rapid consecutive changes.
  let retranslateTimer: ReturnType<typeof setTimeout> | null = null;
  let langReady = false; // No retranslate during the first render (loadDefaults); only user changes emit.
  // Auto-retranslate on language-bar change (debounced). Watches both sides; a change on either triggers.
  $effect(() => {
    const f = fromLang;
    const t = toLang;
    if (!langReady) return; // Skip initial assignment
    if (retranslateTimer) clearTimeout(retranslateTimer);
    retranslateTimer = setTimeout(() => {
      try {
        emitEvent(EventScreenshotRetranslate, {
          session: ScreenshotSessionScreenshot,
          from: f,
          to: t,
        } as ScreenshotRetranslatePayload);
        console.debug(t('log.screenshot_retranslate_emit'), { from: f, to: t });
      } catch (e) {
        console.error(t('log.screenshot_retranslate_failed'), e);
      }
    }, 300);
  });
  // By default only the first two "translation succeeded" cards start expanded; all others (including failures) stay collapsed.
  const successEngines = $derived(
    (result?.translations ?? []).filter((t) => t?.result).map((t) => t.engine),
  );
  const expandedEngines = $derived(new Set(successEngines.slice(0, 2)));

  function recapture() {
    try {
      emitEvent(EventScreenshotRecapture);
    } catch (e) {
      console.error(t('log.screenshotRecaptureFailed'), e);
    }
  }

  // A language picked in either select (issue #53): teach the backend's variant-preference store
  // (the same shared store the input translate window feeds), so picking es-MX once qualifies
  // every later auto-detected Spanish as es-MX. The backend ignores bases and languages without
  // dialects, so every pick is passed through as it is. Called from the selects' own onchange
  // ONLY: swapLangs() and loadDefaults() assign fromLang/toLang directly and must never teach.
  async function learnLangVariant(lang: TranslateLang) {
    await learnFromSelection(lang, LearnLangVariant, (e) =>
      console.error(t('log.screenshotLearnLangVariantFailed'), e),
    );
  }

  // Swap source/target languages (Auto doesn't participate in swaps; landing on the to side is
  // treated as invalid and stays Auto).
  function swapLangs() {
    if (fromLang === TRANSLATE_LANG.Auto) return;
    const tmp = toLang;
    toLang = fromLang;
    fromLang = tmp === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.Auto : tmp;
  }

  function closeWindow() {
    // Clean up image/translation state before closing so the next open is a clean window.
    // Note: closing goes through Window.Close() → the backend's WindowClosing hook (Cancel + Hide),
    // same as the translate window. Don't use Window.Hide() to hide directly — that bypasses the
    // backend hook, leaving the window's hidden state wrong and making Focus ineffective on the
    // next Show (which manifests as being occluded).
    result = null;
    if (imgEl) imgEl.src = '';
    try {
      Window.Close();
    } catch (e) {
      console.error(t('log.screenshotCloseFailed'), e);
    }
  }

  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | null = null;
  function showToast(msg: string) {
    toast = msg;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toast = '';
    }, 1500);
  }

  async function copyText(text: string | undefined) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      showToast(t('common.copied'));
    } catch (e) {
      console.error(t('log.screenshotCopyFailed'), e);
    }
  }

  const off = onEvent(EventScreenshotOCR, (data: ScreenshotResult) => {
    try {
      // Raw event pushed by the backend (backend→frontend), first-hand evidence: what did the backend actually push?
      console.debug(t('log.screenshotLogOcrEvent'), {
        hasImage: !!data.image,
        imagePrefix: (data.image ?? '').slice(0, 80),
        textLen: (data.text ?? '').length,
        error: data.error ?? '',
        translations: (data.translations ?? []).length,
      });
      const incoming = Array.isArray(data.translations) ? data.translations : [];
      // Source text + screenshot are replaced wholesale (every push carries them), so display happens as soon as something is recognized.
      const base = {
        image: data.image,
        text: data.text ?? '',
        to: data.to,
        error: data.error ?? '',
      };
      // translations merges incrementally by engine: same engine overwrites, otherwise appended.
      // This way the backend first pushes empty (source text only), then translations one by one
      // as the frontend displays them one by one.
      const merged = new Map<string, TranslateResult>();
      for (const t of result?.translations ?? []) {
        if (t && typeof t.engine === 'string') merged.set(t.engine, t);
      }
      for (const t of incoming) {
        if (t && typeof t.engine === 'string') merged.set(t.engine, t);
      }
      result = {
        ...base,
        translations: Array.from(merged.values()),
      } as ScreenshotResult;
      if (result.error) {
        console.warn(t('log.screenshotRenderError'), result.error);
      } else {
        console.debug(t('log.screenshotLogRenderResult'), {
          imageLen: (result.image ?? '').length,
          textLen: result.text.length,
          translations: result.translations.length,
          engines: result.translations.map((t) => t.engine),
        });
      }
    } catch (e) {
      console.error(t('log.screenshotRenderOcrFailed'), e);
    }
  });

  // The img's src is synced by a reactive effect (reset whenever result.image changes),
  // avoiding the case where onEvent assigns result before the DOM re-renders (imgEl's bind:this
  // not yet ready) and a direct imgEl.src = ... assignment lands on undefined, leaving the image
  // blank.
  // Also attaches an onerror to diagnose image load failures.
  $effect(() => {
    const url = result?.image;
    if (imgEl && url) {
      // Attach the one-time onerror diagnostic (set only on first attach, avoiding repeated
      // binding on every effect rerun).
      // Note: in the error state (OCR timeout/failure) result is reset wholesale to a new object
      // carrying error; this effect re-runs but src is unchanged (guard below), which shouldn't
      // count as a real load failure — so onerror only logs at warn level and skips reporting in
      // the error state, avoiding false positives on the timeout path.
      if (!imgEl.dataset.ocrErrBound) {
        imgEl.onerror = () => {
          if (result?.error) return; // In the error state the image may be unmounted/remounted; onerror is a side effect, not a real load failure
          console.warn(t('log.screenshotImageLoadFailed'), {
            imageLen: url.length,
            imagePrefix: url.slice(0, 80),
          });
        };
        imgEl.dataset.ocrErrBound = '1';
      }
      // Key: only reset src when it actually changed, avoiding the sporadic onerror caused by
      // each event's new object re-setting the large base64 image's src over and over.
      if (imgEl.getAttribute('src') !== url) {
        imgEl.src = url;
      }
    }
  });

  // Listens for this window's (screenshot) close event: the native red X → the backend's
  // WindowClosing hook broadcasts EventWindowClosing; filtered by window name here, the screenshot
  // and translations are cleared so the next open is a clean window.
  const offClosing = onEvent(EventWindowClosing, (name: string) => {
    if (name !== WindowScreenshot) return;
    result = null;
    if (imgEl) imgEl.src = '';
  });

  onMount(async () => {
    console.debug(t('log.screenshotLogMounted'));
    loadCapability();
    const offEngines = onEvent(EventEnginesChanged, () => {
      loadCapability();
    });
    loadDefaults().then(() => {
      // After loadDefaults sets the initial values, allow language changes to trigger retranslate
      // on the next tick, avoiding a false trigger from the initial assignment.
      setTimeout(() => {
        langReady = true;
      }, 0);
    });
    // The screenshot-translation window is a transient floating window. Its window level is set
    // to MacWindowLevelModalPanel in main.go, so it floats above normal windows by default
    // (AlwaysOnTop not needed). Here we only call SetAlwaysOnTop(true) to pin permanently when the
    // user actively pins; when pin=false it is not called, avoiding pushing the modalPanel level
    // back down to normal (which would lose the floating ability).
    if ($pinnedStore) {
      try {
        await Window.SetAlwaysOnTop(true);
      } catch (e) {
        console.error(t('log.screenshotSetPinFailed'), e);
      }
    }
    return () => {
      off();
      offClosing();
      offEngines();
    };
  });
</script>

<div class="u-surface relative flex h-full flex-col overflow-hidden">
  {#if result}
    <div class="flex min-h-0 flex-1">
      <!-- Left: region screenshot -->
      <div
        class="flex min-w-0 flex-1 items-center justify-center border-r border-[var(--app-border)] p-3"
      >
        <img
          bind:this={imgEl}
          alt="screenshot"
          class="max-h-full max-w-full rounded-lg object-contain shadow-[var(--app-card-shadow)]"
        />
      </div>

      <!-- Right: source text + multi-engine translations -->
      <div class="flex min-w-0 flex-1 flex-col overflow-y-auto p-4">
        <!-- Persistent toolbar: only at the top of the right content area — pin (remembered
             separately from the translate window) + re-screenshot. -->
        <div class="mb-3 flex items-center justify-end gap-2">
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
            class="u-btn u-btn--ghost u-no-drag px-2.5 py-1 text-xs"
            onclick={recapture}
            title={t('screenshot.recapture')}
          >
            {t('screenshot.recapture')}
          </button>
        </div>

        {#if result.error}
          <!-- Recognition/translation failure: stop the spinner and show the error (including
               timeouts) instead of an endless "recognizing" -->
          <div
            class="flex flex-1 flex-col items-center justify-center gap-3 text-[var(--app-muted)]"
          >
            <span class="text-lg font-medium text-[var(--app-fg-strong)]"
              >{t('screenshot.ocrError')}</span
            >
            <span class="max-w-[90%] break-words text-center text-xs opacity-80"
              >{result.error}</span
            >
            <span class="text-xs">{t('screenshot.retryHint')}</span>
          </div>
        {:else if !result.text}
          <!-- Recognizing: OCR hasn't returned the source text yet -->
          <div
            class="flex flex-1 flex-col items-center justify-center gap-3 text-[var(--app-muted)]"
          >
            <span
              class="h-7 w-7 animate-spin rounded-full border-2 border-[var(--app-border)] border-t-[var(--app-accent)]"
            ></span>
            <span class="text-sm">{t('screenshot.recognizing')}</span>
          </div>
        {:else}
          <!-- Language bar: styled like the translate window's; changing language directly triggers a retranslate. -->
          <div class="mb-3 flex items-center justify-center gap-2">
            <select
              class="u-field u-select u-lang-select px-3 py-2 text-sm"
              value={fromLang}
              aria-label={t('translate.from')}
              onchange={(e) => {
                fromLang = e.currentTarget.value as TranslateLang;
                learnLangVariant(fromLang);
              }}
            >
              {#each ALL_TRANSLATE_LANGS as l}
                <option value={l}>{langName(l)}</option>
              {/each}
            </select>

            <button
              class="u-icon-btn u-no-drag"
              aria-label={t('translate.swap')}
              title={t('translate.swap')}
              onclick={swapLangs}
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
              value={toLang}
              aria-label={t('translate.to')}
              onchange={(e) => {
                toLang = e.currentTarget.value as TranslateLang;
                learnLangVariant(toLang);
              }}
            >
              {#each TARGET_TRANSLATE_LANGS as l}
                <!-- issue #52: disabled when no enabled engine can translate into it (backend
                     capability, not a frontend map); the source select is never gated. -->
                <option value={l} disabled={isTargetDisabled(allEngines, l)}>{langName(l)}</option>
              {/each}
            </select>
          </div>

          <div class="mb-3">
            <div
              class="mb-1 flex items-center justify-between text-xs font-medium text-[var(--app-muted)]"
            >
              <span>{t('screenshot.original')}</span>
              <button
                class="u-btn u-btn--ghost p-1"
                title={t('common.copy')}
                aria-label={t('common.copy')}
                onclick={() => copyText(result?.text)}
              >
                <svg
                  class="h-3.5 w-3.5"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                </svg>
              </button>
            </div>
            <div
              class="whitespace-pre-wrap break-words rounded-lg bg-[var(--app-card)] p-3 text-sm leading-relaxed"
            >
              {result.text}
            </div>
          </div>

          {#if result.translations && result.translations.length > 0}
            <div class="flex flex-col gap-3">
              <div class="text-xs font-medium text-[var(--app-muted)]">
                {t('screenshot.result')}
              </div>
              {#each result.translations as tr}
                <!-- requestedTo: result.to is the requested target; each card's own tr.to is where
                     its engine actually translated (issue #44's same-language guard may differ). -->
                <TranslateCard
                  {tr}
                  requestedTo={result.to}
                  expanded={expandedEngines.has(tr.engine)}
                  onCopied={() => showToast(t('common.copied'))}
                />
              {/each}
            </div>
          {:else}
            <!-- Translating: OCR finished, translations not back yet -->
            <div class="mt-4 flex flex-col items-center gap-2 text-[var(--app-muted)]">
              <span
                class="h-6 w-6 animate-spin rounded-full border-2 border-[var(--app-border)] border-t-[var(--app-accent)]"
              ></span>
              <span class="text-sm">{t('screenshot.translating')}</span>
            </div>
          {/if}
        {/if}
      </div>
    </div>
  {:else}
    <div
      class="flex flex-1 items-center justify-center px-6 text-center text-sm text-[var(--app-muted)]"
    >
      {t('screenshot.empty')}
    </div>
  {/if}

  {#if toast}
    <div
      class="pointer-events-none absolute bottom-4 left-1/2 z-50 -translate-x-1/2 rounded-lg bg-black/75 px-3 py-1.5 text-xs text-white shadow-lg"
    >
      {toast}
    </div>
  {/if}
</div>
