<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '../../i18n';
  import {
    GetConfig,
    SaveConfig,
    GetDoubleCopyStatus,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import {
    CheckAccessibility,
    OpenAccessibilitySettings,
    CheckScreenRecording,
    OpenScreenRecordingSettings,
    CheckInputMonitoring,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts';
  import { Dialogs } from '@wailsio/runtime';
  import { isMac as detectMac } from '../../runtime/platform';
  import { onEvent } from '../../runtime';
  import { EventAutoClipboardChanged, EventWindowClosing } from '../../utils/events';
  import { WindowSettings } from '../../constants/window';
  import { createPoller } from '../../utils/permissionPoller';
  import { track } from '../../utils/analytics';

  // Form shape of a single (registration) hotkey (key + enabled state), aligned with the backend's HotkeyEntry.
  type HotkeyEntry = { key: string; enabled: boolean };
  // Form shape of a single exec key, aligned with the backend's ExecKeyEntry (exec keys are a
  // separate category, never mixed with HotkeyEntry).
  type ExecKeyEntry = { key: string; enabled: boolean; fallback: boolean };
  // Hotkey form: editable copy (2 items)
  type HotkeyForm = {
    input: HotkeyEntry;
    screenshot: HotkeyEntry;
  };
  // Exec-key form (separate category): currently only the copy hotkey.
  type ExecKeyForm = {
    copy: ExecKeyEntry;
  };
  // Default hotkeys (kept in sync with the backend's DefaultSettings.Hotkeys).
  const defaultHotkeys: HotkeyForm = {
    input: { key: 'Alt+A', enabled: true },
    screenshot: { key: 'Alt+S', enabled: false },
  };
  // Default exec keys (kept in sync with the backend's DefaultSettings.ExecKeys).
  const defaultExecKeys: ExecKeyForm = {
    copy: { key: 'Cmd+C', enabled: true, fallback: true },
  };

  // Shortcut forms: editable copies (2 hotkeys + the copy exec key)
  // Initial values prefill each shortcut's real default so inputs are never empty before loading.
  let hotkeyForm = $state<HotkeyForm>({
    input: { ...defaultHotkeys.input },
    screenshot: { ...defaultHotkeys.screenshot },
  });
  let execKeyForm = $state<ExecKeyForm>({
    copy: { ...defaultExecKeys.copy },
  });
  // Recording state: the shortcut field name currently capturing a key (null = not recording).
  // Hotkeys and exec keys are two separate categories.
  type RecordableKey = keyof HotkeyForm | keyof ExecKeyForm;
  let recordingKey = $state<RecordableKey | null>(null);

  // When auto-clipboard translation is on, the copy hotkey is automatically disabled (avoiding
  // double-triggering); editing the copy hotkey here is forbidden in that case.
  let copyDisabled = $state(false);

  // Permission cards are only needed on macOS (Accessibility / Screen Recording are both macOS TCC
  // permissions). On Windows the copy hotkey goes through makc calling user32.dll and global hotkeys
  // use RegisterHotKey — neither needs user permission; Linux likewise needs none of these. So the
  // permission block is hidden entirely on non-Mac platforms.
  // Uses the unified platform check (under Wails v3 multi-window, _wails may be missing, so the UA is the fallback).
  const isMac = detectMac();
  console.debug(t('log.shortcutsTabIsMac'), isMac);

  // Accessibility permission state (macOS): true=granted, false=denied, null=detecting/unknown (always true on non-darwin)
  let accGranted = $state<boolean | null>(null);
  let accLoading = $state(false);

  async function loadAccessibility() {
    accLoading = true;
    try {
      accGranted = await CheckAccessibility();
    } catch (e) {
      // Keep the last known state: a failed re-check (every 3 s while Settings is open) must not
      // blank a card that was showing a real answer. Before any answer it stays null ("—").
      console.error(t('log.shortcutCheckAccessibilityFailed'), e);
    } finally {
      accLoading = false;
    }
  }

  async function openAccessibility() {
    try {
      await OpenAccessibilitySettings();
      // After the dialog opens, give the user a moment, then refresh the state once
      setTimeout(loadAccessibility, 800);
    } catch (e) {
      console.error(t('log.shortcutOpenAccessibilityFailed'), e);
    }
  }

  // Screen Recording permission state (screenshot translation depends on it): true=granted, false=denied, null=detecting/unknown (always true on non-darwin)
  let srGranted = $state<boolean | null>(null);
  let srLoading = $state(false);

  async function loadScreenRecording() {
    srLoading = true;
    try {
      srGranted = await CheckScreenRecording();
    } catch (e) {
      console.error(t('log.shortcutCheckScreenRecordingFailed'), e);
    } finally {
      srLoading = false;
    }
  }

  async function openScreenRecording() {
    try {
      await OpenScreenRecordingSettings();
      // After the dialog opens, give the user a moment, then refresh the state once
      setTimeout(loadScreenRecording, 800);
    } catch (e) {
      console.error(t('log.shortcutOpenScreenRecordingFailed'), e);
    }
  }

  // Input Monitoring (translate on double Cmd+C needs it): true=granted, false=denied, null=unknown.
  // macOS shows no prompt for this read; the user switches it on in Privacy & Security.
  let imGranted = $state<boolean | null>(null);

  async function loadInputMonitoring() {
    try {
      imGranted = await CheckInputMonitoring();
    } catch (e) {
      console.error(t('log.shortcutCheckInputMonitoringFailed'), e);
    }
  }

  // Loads all permission states the shortcuts page needs. Data-fetch only; it never changes the
  // expanded/collapsed state, so a manual refresh doesn't forcibly collapse a card the user
  // expanded.
  async function loadShortcutPermissions() {
    await Promise.all([loadAccessibility(), loadScreenRecording(), loadInputMonitoring()]);
    // Set the initial expanded state once, after the first load completes: all granted →
    // collapsed by default, otherwise expanded.
    if (!permInitDone) {
      permInitDone = true;
      permExpanded = !(accGranted === true && srGranted === true);
    }
  }

  // The permission block's expanded state: collapsed by default. Its initial value is decided once
  // by permission state on the page's first load (first data return); subsequent refreshes and
  // manual expand/collapse are user-controlled and never reset.
  let permExpanded = $state(false);
  let permInitDone = false; // whether the initial expanded state has been set; ensures it's set only once

  // Whether all permissions are explicitly granted (Accessibility + Screen Recording); only used for the template's collapse check.
  let permAllGranted = $derived(accGranted === true && srGranted === true);

  // Use e.code for the main key, avoiding macOS Option(Alt) combos turning e.key into a composed
  // character (e.g. Alt+S → 'ß')
  function keyName(e: KeyboardEvent): string {
    if (e.code?.startsWith('Key')) return e.code.slice(3); // KeyS -> S
    if (e.code?.startsWith('Digit')) return e.code.slice(5); // Digit1 -> 1
    if (e.code === 'Space') return 'Space';
    const k = e.key;
    if (k === ' ' || k === 'Spacebar') return 'Space';
    if (k === 'Control' || k === 'Alt' || k === 'Shift' || k === 'Meta') return '';
    if (k.length === 1) return k.toUpperCase();
    return k;
  }

  function onHotkeyKeydown(e: KeyboardEvent) {
    if (!recordingKey) return;
    e.preventDefault();
    if (e.key === 'Escape') {
      recordingKey = null;
      return;
    }
    const mods: string[] = [];
    if (e.altKey) mods.push('Alt');
    if (e.ctrlKey) mods.push('Ctrl');
    if (e.metaKey) mods.push('Cmd');
    if (e.shiftKey) mods.push('Shift');
    const main = keyName(e);
    if (!main) return; // Modifier only; wait for the main key
    const combo = [...mods, main].join('+');
    const k = recordingKey;
    recordingKey = null; // Clear first, preventing a duplicate keydown from the combo writing again
    if (k) {
      if (k === 'copy') {
        execKeyForm.copy.key = combo;
      } else {
        hotkeyForm[k].key = combo;
      }
    }
  }

  function startRecord(key: keyof HotkeyForm | 'copy') {
    // When the copy hotkey is locked by auto-clipboard (copyDisabled), forbid recording — a
    // backstop for the disabled attribute, preventing a click-through in edge cases from
    // modifying the copy hotkey.
    if (key === 'copy' && copyDisabled) return;
    recordingKey = key;
  }

  // Issue #14: while the Settings window is visible, re-read the permissions every 3 s so a change
  // made in System Settings shows up without reopening the page. One timer, and a slow check is
  // waited for (utils/permissionPoller). The first load still decides the block's open/closed
  // state once (loadShortcutPermissions); a re-check only updates the cards.
  async function pollPermissions() {
    await Promise.all([loadShortcutPermissions(), refreshDoubleCopyStatus()]);
  }
  const poller = createPoller(pollPermissions);

  onMount(() => {
    loadShortcuts();
    loadShortcutPermissions();
    // Poll only while the window is on screen. The webview reports hidden/visible when Kai hides or
    // shows it; the red X only hides the window (its close hook), so that broadcast stops polling
    // too, and the next focus or visibility change resumes it with an immediate check.
    const resume = () => {
      if (document.visibilityState === 'visible') poller.start(true);
    };
    const onVisibility = () => (document.visibilityState === 'visible' ? resume() : poller.stop());
    if (document.visibilityState === 'visible') poller.start();
    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('focus', resume);
    const offClosing = onEvent(EventWindowClosing, (name: string) => {
      if (name === WindowSettings) poller.stop();
    });
    // When the translate window toggles auto-clipboard, disable/restore this page's copy-hotkey controls in real time.
    const offAuto = onEvent(EventAutoClipboardChanged, (enabled: boolean) => {
      copyDisabled = enabled;
    });
    return () => {
      offAuto();
      offClosing();
      poller.stop();
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('focus', resume);
    };
  });

  // Cross-window events alone may not arrive (multi-window isolation), so when the window regains
  // focus (e.g. switching back from the translate window to settings), read GetConfig once more to
  // make sure the auto-clipboard state is synced and the copy-hotkey controls correctly enter/exit
  // the disabled state.
  async function refreshCopyDisabled() {
    try {
      const cfg = await GetConfig();
      if (cfg) copyDisabled = !!cfg.auto_clipboard;
    } catch (e) {
      console.error(t('log.shortcutLoadConfigFailed'), e);
    }
  }

  async function loadShortcuts() {
    try {
      const cfg = await GetConfig();
      if (cfg) {
        const hk = cfg.hotkeys ?? {};
        hotkeyForm = {
          input: { key: hk.input?.key ?? '', enabled: hk.input?.enabled ?? false },
          screenshot: { key: hk.screenshot?.key ?? '', enabled: hk.screenshot?.enabled ?? false },
        };
        const ek = cfg.execkeys ?? {};
        execKeyForm = {
          copy: {
            key: ek.copy?.key ?? '',
            enabled: ek.copy?.enabled ?? false,
            fallback: ek.copy?.fallback ?? true,
          },
        };
        copyDisabled = !!cfg.auto_clipboard;
      }
    } catch (e) {
      console.error(t('log.shortcutLoadConfigFailed'), e);
    }
  }

  async function saveShortcuts() {
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      const next = {
        ...cfg,
        hotkeys: {
          input: hotkeyForm.input,
          screenshot: hotkeyForm.screenshot,
        },
        execkeys: { copy: execKeyForm.copy },
      };
      await SaveConfig(next as any);
      // Desktop app: use Wails v3's native info dialog, consistent with the failure error dialog and more prominent
      await Dialogs.Info({
        Title: t('settings.hkSavedTitle'),
        Message: t('settings.hkSaved'),
      });
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // Desktop app: use Wails v3's native error dialog, more prominent than inline text
      await Dialogs.Error({
        Title: t('settings.hkSaveErrorTitle'),
        Message: msg,
      });
    }
  }

  // --- Translate on double Cmd+C (issue #199, moved here by ADR-0003 / #122) ---
  // A passive listen-only trigger with its own on/off switch, not a recordable shortcut. OFF by
  // default (it needs the Input Monitoring permission); the Go side owns the default and the
  // listener's state, this mirrors them.
  let doubleCopy = $state(false);
  let doubleCopyMissingPermission = $state(false);
  // The listener caches "missing_permission" from when it started, so it stays set after the user
  // grants Input Monitoring. The live check wins once it has an answer; the cached status only
  // covers the time before one.
  const doubleCopyNeedsPermission = $derived(
    doubleCopy && (imGranted === false || (imGranted === null && doubleCopyMissingPermission)),
  );

  async function refreshDoubleCopyStatus() {
    try {
      doubleCopyMissingPermission = (await GetDoubleCopyStatus()) === 'missing_permission';
    } catch {
      doubleCopyMissingPermission = false;
    }
  }

  onMount(async () => {
    try {
      const cfg = await GetConfig();
      if (cfg) doubleCopy = cfg.double_copy_translate ?? false;
    } catch {
      /* Ignore read failures; fall back to the default (off) */
    }
    await refreshDoubleCopyStatus();
  });

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
      console.error(t('log.shortcutSaveDoubleCopyFailed'), err);
    }
    await refreshDoubleCopyStatus();
  }
</script>

<svelte:window onkeydown={onHotkeyKeydown} onfocus={refreshCopyDisabled} />

<header class="mb-6">
  <h1 class="text-2xl font-semibold">{t('settings.shortcutsTitle')}</h1>
  <p class="u-muted mt-1 text-sm">{t('settings.shortcutsHint')}</p>
</header>

<!-- Permissions required by shortcuts: only needed on macOS (Accessibility + Screen Recording are both macOS TCC permissions) -->
{#if isMac}
  <div class="u-card u-card--panel mb-5 p-5">
    {#if permAllGranted && !permExpanded}
      <!-- Collapsed state: all granted collapses by default, showing only the summary line -->
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium">{t('settings.permTitle')}</span>
          <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <button class="u-btn u-btn--ghost px-3 py-1.5 text-sm" onclick={loadShortcutPermissions}>
            {t('settings.accRefresh')}
          </button>
          <button
            class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
            onclick={() => (permExpanded = true)}
          >
            {t('settings.permExpand')}
          </button>
        </div>
      </div>
    {:else}
      <!-- Expanded state: details + refresh/collapse -->
      <div class="mb-1 flex items-center justify-between gap-4">
        <div class="text-sm font-medium">{t('settings.permTitle')}</div>
        <div class="flex shrink-0 items-center gap-2">
          {#if permAllGranted}
            <button
              class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
              onclick={() => (permExpanded = false)}
            >
              {t('settings.permCollapse')}
            </button>
          {/if}
          <button class="u-btn u-btn--ghost px-3 py-1.5 text-sm" onclick={loadShortcutPermissions}>
            {t('settings.accRefresh')}
          </button>
        </div>
      </div>
      <p class="u-muted mb-4 text-xs">{t('settings.permHint')}</p>

      <div class="space-y-4">
        <!-- Accessibility -->
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="text-sm font-medium">{t('settings.permAccessibility')}</div>
            <p class="u-muted text-xs">{t('settings.permAccessibilityHint')}</p>
            {#if accGranted === false}
              <p class="u-text-warn mt-1 text-xs" data-testid="accessibility-missing">
                {t('settings.permAccessibilityMissing')}
              </p>
            {/if}
          </div>
          <div class="flex shrink-0 items-center gap-2">
            {#if accGranted === null}
              <span class="u-muted text-sm">{accLoading ? t('common.loading') : '—'}</span>
            {:else if accGranted}
              <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
            {:else}
              <span class="u-text-warn text-sm font-medium">{t('settings.accDenied')}</span>
            {/if}
            <button class="u-btn u-btn--primary px-3 py-1.5 text-sm" onclick={openAccessibility}>
              {t('settings.accOpen')}
            </button>
          </div>
        </div>

        <!-- Screen Recording: screenshot translation depends on it -->
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="text-sm font-medium">{t('settings.permScreenRecording')}</div>
            <p class="u-muted text-xs">{t('settings.permScreenRecordingHint')}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            {#if srGranted === null}
              <span class="u-muted text-sm">{srLoading ? t('common.loading') : '—'}</span>
            {:else if srGranted}
              <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
            {:else}
              <span class="u-text-warn text-sm font-medium">{t('settings.accDenied')}</span>
            {/if}
            <button class="u-btn u-btn--primary px-3 py-1.5 text-sm" onclick={openScreenRecording}>
              {t('settings.accOpen')}
            </button>
          </div>
        </div>

        <!-- Input Monitoring: translate on double Cmd+C depends on it (no button: there is no prompt to raise) -->
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="text-sm font-medium">{t('settings.permInputMonitoring')}</div>
            <p class="u-muted text-xs">{t('settings.permInputMonitoringHint')}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            {#if imGranted === null}
              <span class="u-muted text-sm">—</span>
            {:else if imGranted}
              <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
            {:else}
              <span class="u-text-warn text-sm font-medium">{t('settings.accDenied')}</span>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}

<div class="u-card u-card--panel space-y-5 p-6">
  {#each [{ key: 'input', label: t('settings.hkInput') }, { key: 'screenshot', label: t('settings.hkScreenshot') }] satisfies { key: keyof HotkeyForm; label: string }[] as row}
    <div class="flex items-center justify-between gap-4">
      <label class="text-sm font-medium" for={'hk-' + row.key}>{row.label}</label>
      <div class="flex items-center gap-2">
        {#if recordingKey === row.key}
          <span class="u-field w-56 px-3 py-1.5 text-sm u-text-warn"
            >{t('settings.hkRecording')}</span
          >
        {:else}
          <input
            id={'hk-' + row.key}
            class="u-field w-56 px-3 py-1.5 text-sm"
            placeholder={defaultHotkeys[row.key].key}
            bind:value={hotkeyForm[row.key].key}
          />
        {/if}
        <button
          type="button"
          class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
          class:is-active={recordingKey === row.key}
          onclick={() => startRecord(row.key)}>{t('settings.hkRecord')}</button
        >
        <label class="u-switch" aria-label={t('settings.enabled')}>
          <input type="checkbox" bind:checked={hotkeyForm[row.key].enabled} />
          <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
        </label>
      </div>
    </div>
  {/each}
  <div class="flex items-center justify-between gap-4 border-t pt-4">
    <label class="text-sm font-medium" for="hk-copy">{t('settings.hkCopy')}</label>
    <div class="flex flex-wrap items-center gap-2">
      {#if recordingKey === 'copy'}
        <span class="u-field w-56 px-3 py-1.5 text-sm u-text-warn">{t('settings.hkRecording')}</span
        >
      {:else}
        <input
          id="hk-copy"
          class="u-field w-56 px-3 py-1.5 text-sm"
          placeholder={defaultExecKeys.copy.key}
          bind:value={execKeyForm.copy.key}
          disabled={copyDisabled}
        />
      {/if}
      <button
        type="button"
        class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
        class:is-active={recordingKey === 'copy'}
        disabled={copyDisabled}
        onclick={() => startRecord('copy')}>{t('settings.hkRecord')}</button
      >
      <label
        class="u-switch"
        class:is-disabled={copyDisabled}
        aria-label={t('settings.hkCopyEnable')}
      >
        <input type="checkbox" bind:checked={execKeyForm.copy.enabled} disabled={copyDisabled} />
        <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
        <span class="text-xs">{t('settings.hkCopyEnable')}</span>
      </label>
      <label
        class="u-switch"
        class:is-disabled={copyDisabled}
        aria-label={t('settings.hkCopyFallback')}
      >
        <input type="checkbox" bind:checked={execKeyForm.copy.fallback} disabled={copyDisabled} />
        <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
        <span class="text-xs">{t('settings.hkCopyFallbackShort')}</span>
      </label>
    </div>
  </div>
  {#if copyDisabled}
    <p class="u-muted -mt-2 text-xs">{t('settings.copyKeyDisabledHint')}</p>
  {/if}
  <p class="u-muted text-xs">{t('settings.hkFormatHint')}</p>
  <div class="flex justify-end pt-1">
    <button class="u-btn u-btn--primary px-4 py-1.5 text-sm" onclick={saveShortcuts}>
      {t('settings.hkSave')}
    </button>
  </div>
</div>

<!-- Translate on double Cmd+C: a passive trigger with its own switch (ADR-0003 / #122) -->
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
  {#if doubleCopyNeedsPermission}
    <p class="mt-3 text-xs" role="alert" data-testid="double-copy-permission">
      {t('settings.doubleCopyPermission')}
    </p>
  {/if}
</div>
