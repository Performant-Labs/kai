<script lang="ts">
  import { t } from '../../i18n';
  import HistoryList from '../HistoryList.svelte';

  // The same list as the History window (issue #60), so the two cannot drift apart.
</script>

<div class="flex h-[calc(100vh-4rem)] min-h-0 flex-col">
  <HistoryList title={t('settings.historyTitle')} />
</div>
