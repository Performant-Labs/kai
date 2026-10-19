<script lang="ts">
  import { t } from '../i18n';
  import { Window } from '../runtime';
  import { rootStyle } from '../stores/theme';
  import { rootStyleToStyle } from '../utils/style';
  import HistoryList from './HistoryList.svelte';

  // The History window (issue #60): its own window, like Settings, so the translate window keeps its
  // size and the history can stay open beside it. The window only hides when it is closed (the Go
  // side's WindowClosing hook), so Esc asks it to close and it comes back with its filters and scroll.
  function close() {
    Window.Close().catch((e) => console.error(t('log.historyUseFailed'), e));
  }
</script>

<div class="u-surface flex h-full w-full flex-col p-6" style={rootStyleToStyle($rootStyle)}>
  <HistoryList title={t('history.title')} onescape={close} />
</div>
