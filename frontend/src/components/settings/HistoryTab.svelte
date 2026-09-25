<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../../i18n';
  import {
    GetHistory,
    CountHistory,
    DeleteHistory,
    ClearHistory,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/historywrapper.ts';
  import type { HistoryItem } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import { Dialogs } from '@wailsio/runtime';

  let history = $state<HistoryItem[]>([]);
  // History pagination and search
  let historyKeyword = $state('');
  let historyPage = $state(1);
  const historyPageSize = 20;
  let historyTotal = $state(0);
  let historyLoading = $state(false);

  // History loading with pagination + search: keyword comes from the input, offset is derived
  // from the current page number.
  async function loadHistory() {
    if (historyLoading) return;
    historyLoading = true;
    try {
      const kw = historyKeyword.trim();
      const offset = (historyPage - 1) * historyPageSize;
      historyTotal = await CountHistory(kw);
      const rows = (await GetHistory(kw, offset, historyPageSize)) ?? [];
      history = rows;
      // Re-clamp the page number: after a search it may be out of range (e.g. after a delete or a
      // filter change); fall back to the last page.
      const totalPages = Math.max(1, Math.ceil(historyTotal / historyPageSize));
      if (historyPage > totalPages) {
        historyPage = totalPages;
        const fixedOffset = (historyPage - 1) * historyPageSize;
        history = (await GetHistory(kw, fixedOffset, historyPageSize)) ?? [];
      }
    } catch (e) {
      console.error(t('log.historyLoadFailed'), e);
      history = [];
      await Dialogs.Error({
        Title: t('settings.historyOpErrorTitle'),
        Message: t('settings.historyLoadError'),
      });
    } finally {
      historyLoading = false;
    }
  }

  // Search input: debounce 300ms, then reload from page 1.
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  function onHistorySearch() {
    historyPage = 1;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => loadHistory(), 300);
  }

  function goPrevPage() {
    if (historyPage > 1) {
      historyPage--;
      loadHistory();
    }
  }
  function goNextPage() {
    const totalPages = Math.max(1, Math.ceil(historyTotal / historyPageSize));
    if (historyPage < totalPages) {
      historyPage++;
      loadHistory();
    }
  }

  async function deleteHistory(id: number) {
    try {
      await DeleteHistory(id);
      await loadHistory();
    } catch (e) {
      console.error(t('log.historyDeleteFailed'), e);
      await Dialogs.Error({
        Title: t('settings.historyOpErrorTitle'),
        Message: t('settings.historyDeleteError'),
      });
    }
  }

  async function clearHistory() {
    try {
      await ClearHistory();
      historyPage = 1;
      await loadHistory();
      await Dialogs.Info({
        Title: t('settings.historyOkTitle'),
        Message: t('settings.historyCleared'),
      });
    } catch (e) {
      console.error(t('log.historyClearFailed'), e);
      await Dialogs.Error({
        Title: t('settings.historyOpErrorTitle'),
        Message: t('settings.historyClearError'),
      });
    }
  }

  // Load on first mount
  onMount(() => {
    loadHistory();
  });
</script>

<header class="mb-6 flex items-end justify-between">
  <div>
    <h1 class="text-2xl font-semibold">{t('settings.historyTitle')}</h1>
  </div>
  {#if historyTotal > 0}
    <button class="u-btn u-btn--danger px-4 py-1.5 text-sm" onclick={clearHistory}>
      {t('settings.historyClear')}
    </button>
  {/if}
</header>

<div class="u-card u-card--panel flex flex-col p-0 overflow-hidden">
  <!-- Search box -->
  <div class="border-b p-4">
    <input
      class="u-field w-full px-3 py-2 text-sm"
      type="search"
      placeholder={t('settings.historySearchPlaceholder')}
      bind:value={historyKeyword}
      oninput={onHistorySearch}
    />
  </div>

  {#if history.length === 0}
    <div class="flex flex-col items-center justify-center py-16 text-center">
      <p class="u-muted text-sm">{t('settings.historyEmpty')}</p>
    </div>
  {:else}
    <ul class="u-divide divide-y overflow-y-auto" style="max-height: 56vh;">
      {#each history as h}
        <li class="u-card u-card--row flex items-start gap-4 px-5 py-3">
          <div class="min-w-0 flex-1 space-y-2">
            <div class="space-y-1">
              <p class="u-muted text-xs">{t('settings.historySource')}</p>
              <p class="break-words text-sm">{h.text}</p>
            </div>
            {#if h.result}
              <div class="u-card u-card--row space-y-1 border-l-2 px-3 py-2">
                <p class="u-muted text-xs">{t('settings.historyResult')}</p>
                <p class="break-words text-sm u-text-accent whitespace-pre-wrap">{h.result}</p>
              </div>
            {/if}
            <p class="u-muted text-xs">
              {langName(h.from_lang)} → {langName(h.to_lang)}{#if h.engine}
                · {engineName(h.engine)}{/if}
            </p>
          </div>
          <button
            class="u-btn u-btn--ghost ml-4 shrink-0 px-3 py-1 text-xs"
            onclick={() => deleteHistory(h.id)}
          >
            {t('common.delete')}
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  <!-- Pagination bar -->
  {#if historyTotal > 0}
    <div class="flex items-center justify-between gap-3 border-t p-3 text-sm">
      <span class="u-muted">
        {t('settings.historyPageInfo', {
          cur: historyPage,
          total: Math.max(1, Math.ceil(historyTotal / historyPageSize)),
          count: historyTotal,
        })}
      </span>
      <div class="flex items-center gap-2">
        <button
          class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
          disabled={historyPage <= 1}
          onclick={goPrevPage}
        >
          {t('settings.historyPrev')}
        </button>
        <button
          class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
          disabled={historyPage >= Math.max(1, Math.ceil(historyTotal / historyPageSize))}
          onclick={goNextPage}
        >
          {t('settings.historyNext')}
        </button>
      </div>
    </div>
  {/if}
</div>
