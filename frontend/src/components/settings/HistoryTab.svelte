<script lang="ts">
  import { t } from '../../i18n';
  import HistoryList from '../HistoryList.svelte';

  // Issue #60: the Settings history tab shows the SAME list component as the history window
  // ("one list, one set of rules", the issue's recommended decision) — not a second
  // implementation. "Use" from here broadcasts like anywhere else; the translate window
  // brings itself forward if it was hidden (see TranslateWindow's EventHistoryUse handler).
</script>

<header class="mb-6 flex items-end justify-between">
  <div>
    <h1 class="text-2xl font-semibold">{t('history.title')}</h1>
  </div>
</header>

<div class="u-card u-card--panel flex h-[56vh] flex-col overflow-hidden p-0">
  <div class="min-h-0 flex-1">
    <HistoryList />
  </div>
</div>
