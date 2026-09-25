<script lang="ts">
  import { Window, emitEvent } from '../runtime';
  import { t } from '../i18n';
  import { isDark } from '../stores/theme';
  import { EventWindowClosing } from '../utils/events';
  import { isMac } from '../runtime/platform';

  let { onClose, windowName }: { onClose?: () => void; windowName?: string } = $props();

  // Platform checks all go through runtime/platform (under Wails v3 multi-window, _wails may be
  // missing, so the UA is the fallback).
  const isMacPlatform = isMac();
  console.debug(t('log.titleBarIsMac'), isMacPlatform);

  function minimize() {
    Window.Minimise();
  }
  function toggleMax() {
    Window.ToggleMaximise();
  }
  function close() {
    // Custom close behavior (e.g. the screenshot window only hides, it isn't destroyed); otherwise the standard close flow.
    if (onClose) {
      onClose();
      return;
    }
    emitEvent(EventWindowClosing, windowName);
    Window.Close();
  }
</script>

<div
  class="u-titlebar u-drag flex h-9 items-center select-none px-2 {isMacPlatform
    ? 'justify-start'
    : 'justify-end'}"
  class:dark={$isDark}
>
  {#if isMacPlatform}
    <div class="u-no-drag flex gap-2 pr-2">
      <button class="u-traffic u-traffic--close" aria-label={t('titlebar.close')} onclick={close}
      ></button>
      <button
        class="u-traffic u-traffic--min"
        aria-label={t('titlebar.minimize')}
        onclick={minimize}
      ></button>
      <button
        class="u-traffic u-traffic--max"
        aria-label={t('titlebar.maximize')}
        onclick={toggleMax}
      ></button>
    </div>
  {:else}
    <div class="u-no-drag flex gap-0.5">
      <button class="u-winbtn" aria-label={t('titlebar.minimize')} onclick={minimize}>—</button>
      <button class="u-winbtn" aria-label={t('titlebar.maximize')} onclick={toggleMax}>▢</button>
      <button class="u-winbtn u-winbtn--close" aria-label={t('titlebar.close')} onclick={close}
        >✕</button
      >
    </div>
  {/if}
</div>
