<script lang="ts">
  import { t } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { rootStyleToStyle } from '../utils/style';
  import { Window } from '@wailsio/runtime';
  import HistoryList from './HistoryList.svelte';

  // The history screen's window half (issue #60): everything list-specific lives in
  // HistoryList; what lives here is only what being a window means — the title, and Esc
  // closing the screen. Closing goes through Window.Close() → the backend's WindowClosing hook
  // (Cancel + Hide, same as Settings): that hook is what tells the translate window to put its
  // always-on-top pin back (it was lowered so this window could show in front of it). A bare
  // Window.Hide() would skip the broadcast and leave the translate window unpinned.
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    e.preventDefault();
    Window.Close().catch((e) => console.error(t('log.historyCloseFailed'), e));
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="u-surface flex h-screen flex-col" style={rootStyleToStyle($rootStyle)}>
  <main class="flex h-full min-h-0 flex-col p-4">
    <h1 class="pb-3 text-lg font-semibold">{t('history.title')}</h1>
    <div class="min-h-0 flex-1">
      <HistoryList />
    </div>
  </main>
</div>
