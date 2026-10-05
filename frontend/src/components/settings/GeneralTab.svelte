<script lang="ts">
  import { onMount } from 'svelte';
  import { Clipboard } from '@wailsio/runtime';
  import { t, locale, resolveLang } from '../../i18n';
  import { userLang } from '../../stores/ui';
  import { themeMode, setTheme } from '../../stores/theme';
  import {
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import {
    GetVersion,
    IsDevBuild,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts';
  import { Lang, type LangCode } from '../../constants/lang';
  import { THEME, type ThemeMode } from '../../constants/theme';
  import { track } from '../../utils/analytics';
  import { fontSize, saveFontSize } from '../../stores/fontSize';
  import { DEFAULT_FONT_SIZE, FONT_SIZE_STEPS, stepFontSize } from '../../constants/fontSize';

  let { curLang = $bindable<LangCode>(Lang.ZHCN) }: { curLang: LangCode } = $props();

  // Anonymous analytics toggle: initial value read from config; changes persist via SaveConfig
  // (the Go-side analytics uses this to decide whether to report).
  let analyticsEnabled = $state(false);
  // Auto-switch source language (issue #200): ON by default, and a config without the value reads as
  // on; the Go side owns the default, this only mirrors it.
  let autoSwitchSource = $state(true);

  // About card (issue #41): the installed version, whether it is a development build, and a short-lived
  // "Copied" on the copy button.
  let appVersion = $state('');
  let devBuild = $state(false);
  let versionCopied = $state(false);

  async function loadAbout() {
    try {
      appVersion = await GetVersion();
      devBuild = await IsDevBuild();
    } catch {
      /* Leave the card blank; the rest of the tab does not depend on it */
    }
  }

  async function copyVersion() {
    try {
      await Clipboard.SetText(`Kai ${appVersion}`);
      versionCopied = true;
      setTimeout(() => (versionCopied = false), 1500);
    } catch (e) {
      console.error(t('log.generalCopyVersionFailed'), e);
    }
  }

  const themeOptions = $derived.by<{ mode: ThemeMode; label: string }[]>(() => [
    { mode: THEME.Auto, label: t('settings.themeAuto') },
    { mode: THEME.Light, label: t('settings.themeLight') },
    { mode: THEME.Dark, label: t('settings.themeDark') },
  ]);

  onMount(async () => {
    // The About card is independent of the config read below: a failure of either leaves the other.
    void loadAbout();
    try {
      const cfg = await GetConfig();
      if (cfg) analyticsEnabled = cfg.analytics_enabled ?? false;
      if (cfg) autoSwitchSource = cfg.auto_switch_source ?? true;
    } catch {
      /* Ignore read failures; fall back to the default (off) */
    }
  });

  async function changeLang(l: LangCode) {
    curLang = l; // the dropdown highlight keeps the original mode (auto/zh-CN/en-US)
    userLang.set(l); // sync the user mode so the dropdown still selects this item after a reload
    locale.set(resolveLang(l)); // resolve the effective i18n language immediately, avoiding raw keys when auto
    try {
      const cfg = (await GetConfig()) ?? ({ language: l } as any);
      await SaveConfig({ ...cfg, language: l });
    } catch (e) {
      console.error(t('log.generalSaveLangFailed'), e);
    }
  }

  // Text size (issue #195): stepped one notch at a time; saveFontSize applies it at once and the
  // backend broadcasts it to the other windows.
  const atSmallest = $derived($fontSize <= FONT_SIZE_STEPS[0]);
  const atLargest = $derived($fontSize >= FONT_SIZE_STEPS[FONT_SIZE_STEPS.length - 1]);

  async function changeFontSize(next: number) {
    await saveFontSize(next);
    track('feature_toggled', { feature: 'font_size', value: next });
  }

  async function changeTheme(m: ThemeMode) {
    await setTheme(m);
  }

  async function toggleAnalytics(e: Event) {
    const enabled = (e.target as HTMLInputElement).checked;
    analyticsEnabled = enabled;
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, analytics_enabled: enabled });
      track('feature_toggled', { feature: 'anonymous_analytics', enabled });
    } catch (err) {
      console.error(t('log.generalSaveAnalyticsFailed'), err);
    }
  }

  async function toggleAutoSwitchSource(e: Event) {
    const enabled = (e.target as HTMLInputElement).checked;
    autoSwitchSource = enabled;
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, auto_switch_source: enabled });
      track('feature_toggled', { feature: 'auto_switch_source', enabled });
    } catch (err) {
      console.error(t('log.generalSaveAutoSwitchFailed'), err);
    }
  }
</script>

<header class="mb-8">
  <h1 class="text-2xl font-semibold">{t('settings.generalTitle')}</h1>
  <p class="u-muted mt-1 text-sm">{t('settings.interface')}</p>
</header>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.language')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.languageHint')}</p>
  <div class="flex items-center">
    <select
      id="lang-sel"
      class="u-field u-select w-full max-w-[240px] px-3 py-2 text-sm"
      value={curLang}
      onchange={(e) => changeLang((e.target as HTMLSelectElement).value as LangCode)}
    >
      <option value={Lang.Auto}>{t('lang.auto')}</option>
      <option value={Lang.ZHCN}>{t('lang.zh-CN')}</option>
      <option value={Lang.ENUS}>{t('lang.en')}</option>
    </select>
  </div>
</div>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-3 text-sm font-medium">{t('settings.theme')}</div>
  <div class="u-segment">
    {#each themeOptions as opt}
      <button
        class="u-segment__item"
        class:is-active={$themeMode === opt.mode}
        onclick={() => changeTheme(opt.mode)}
      >
        {opt.label}
      </button>
    {/each}
  </div>
</div>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.fontSize')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.fontSizeHint')}</p>
  <div class="flex items-center gap-2">
    <button
      class="u-btn u-btn--ghost u-tooltip px-3 py-1.5 text-sm"
      data-testid="font-size-smaller"
      aria-label={t('settings.fontSizeSmaller')}
      data-tooltip={t('settings.fontSizeSmaller')}
      disabled={atSmallest}
      onclick={() => changeFontSize(stepFontSize($fontSize, -1))}
    >
      A−
    </button>
    <span class="min-w-[3.5rem] text-center text-sm font-medium" data-testid="font-size-value"
      >{$fontSize}%</span
    >
    <button
      class="u-btn u-btn--ghost u-tooltip px-3 py-1.5 text-sm"
      data-testid="font-size-larger"
      aria-label={t('settings.fontSizeLarger')}
      data-tooltip={t('settings.fontSizeLarger')}
      disabled={atLargest}
      onclick={() => changeFontSize(stepFontSize($fontSize, 1))}
    >
      A+
    </button>
    <button
      class="u-btn u-btn--ghost u-tooltip px-3 py-1.5 text-sm"
      data-testid="font-size-reset"
      aria-label={t('settings.fontSizeReset')}
      data-tooltip={t('settings.fontSizeReset')}
      disabled={$fontSize === DEFAULT_FONT_SIZE}
      onclick={() => changeFontSize(DEFAULT_FONT_SIZE)}
    >
      ↺
    </button>
  </div>
</div>

<div class="u-card u-card--panel p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.analytics')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.analyticsHint')}</p>
  <label class="u-switch" aria-label={t('settings.analytics')}>
    <input type="checkbox" checked={analyticsEnabled} onchange={toggleAnalytics} />
    <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
  </label>
</div>

<div class="u-card u-card--panel mt-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.autoSwitchSource')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.autoSwitchSourceHint')}</p>
  <label
    class="u-switch"
    aria-label={t('settings.autoSwitchSource')}
    title={t('settings.autoSwitchSourceHint')}
  >
    <input type="checkbox" checked={autoSwitchSource} onchange={toggleAutoSwitchSource} />
    <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
  </label>
</div>

<div class="u-card u-card--panel mt-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.about')}</div>
  <div class="flex items-center gap-3 text-sm">
    <span class="u-muted">{t('settings.aboutVersion')}</span>
    <span class="font-medium" data-testid="app-version">{appVersion}</span>
    {#if devBuild}
      <span class="u-muted text-xs" data-testid="app-dev-build">{t('settings.aboutDevBuild')}</span>
    {/if}
    <button
      class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
      data-testid="app-version-copy"
      aria-label={t('settings.aboutCopy')}
      disabled={!appVersion}
      onclick={copyVersion}
    >
      {versionCopied ? t('common.copied') : t('settings.aboutCopy')}
    </button>
  </div>
</div>
