<script lang="ts">
  import { onMount } from 'svelte';
  import { t, locale, resolveLang } from '../../i18n';
  import { userLang } from '../../stores/ui';
  import { themeMode, setTheme } from '../../stores/theme';
  import {
    GetConfig,
    SaveConfig,
    GetDoubleCopyStatus,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { Lang, type LangCode } from '../../constants/lang';
  import { THEME, type ThemeMode } from '../../constants/theme';
  import { track } from '../../utils/analytics';

  let { curLang = $bindable<LangCode>(Lang.ZHCN) }: { curLang: LangCode } = $props();

  // Anonymous analytics toggle: initial value read from config; changes persist via SaveConfig
  // (the Go-side analytics uses this to decide whether to report).
  let analyticsEnabled = $state(false);
  // Auto-switch source language (issue #200): ON by default, and a config without the value reads as
  // on; the Go side owns the default, this only mirrors it.
  let autoSwitchSource = $state(true);
  // Translate on double Cmd+C (issue #199): OFF by default (it needs the Input Monitoring
  // permission); the Go side owns the default and the listener's state, this mirrors them.
  let doubleCopy = $state(false);
  let doubleCopyMissingPermission = $state(false);

  async function refreshDoubleCopyStatus() {
    try {
      doubleCopyMissingPermission = (await GetDoubleCopyStatus()) === 'missing_permission';
    } catch {
      doubleCopyMissingPermission = false;
    }
  }

  const themeOptions = $derived.by<{ mode: ThemeMode; label: string }[]>(() => [
    { mode: THEME.Auto, label: t('settings.themeAuto') },
    { mode: THEME.Light, label: t('settings.themeLight') },
    { mode: THEME.Dark, label: t('settings.themeDark') },
  ]);

  onMount(async () => {
    try {
      const cfg = await GetConfig();
      if (cfg) analyticsEnabled = cfg.analytics_enabled ?? false;
      if (cfg) autoSwitchSource = cfg.auto_switch_source ?? true;
      if (cfg) doubleCopy = cfg.double_copy_translate ?? false;
    } catch {
      /* Ignore read failures; fall back to the default (off) */
    }
    await refreshDoubleCopyStatus();
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

  async function toggleDoubleCopy(e: Event) {
    const enabled = (e.target as HTMLInputElement).checked;
    doubleCopy = enabled;
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      // SaveConfig makes the backend start or stop the listener (and ask for the permission when
      // this is the user switching it on), so the status is read after it.
      await SaveConfig({ ...cfg, double_copy_translate: enabled });
      track('feature_toggled', { feature: 'double_copy_translate', enabled });
    } catch (err) {
      console.error(t('log.generalSaveDoubleCopyFailed'), err);
    }
    await refreshDoubleCopyStatus();
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

<div class="u-card u-card--panel p-5">
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
  <div class="mb-1 text-sm font-medium">{t('settings.doubleCopy')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.doubleCopyHint')}</p>
  <label
    class="u-switch"
    aria-label={t('settings.doubleCopy')}
    title={t('settings.doubleCopyHint')}
  >
    <input type="checkbox" checked={doubleCopy} onchange={toggleDoubleCopy} />
    <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
  </label>
  {#if doubleCopy && doubleCopyMissingPermission}
    <p class="mt-3 text-xs" role="alert" data-testid="double-copy-permission">
      {t('settings.doubleCopyPermission')}
    </p>
  {/if}
</div>
