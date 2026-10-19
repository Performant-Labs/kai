<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { rootStyleToStyle } from '../utils/style';
  import { emitEvent } from '../runtime';
  import { Clipboard, Dialogs } from '@wailsio/runtime';
  import {
    GetHistory,
    CountHistory,
    DeleteHistory,
    ClearHistory,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/historywrapper.ts';
  import { EventHistoryUse } from '../utils/events';
  import {
    CLAMP_CHARS,
    canLoadMore,
    clampText,
    engineOptions,
    groupDays,
    pairOptions,
    rowTimeLabel,
    usePayload,
    type HistoryRow,
  } from '../utils/historyList.ts';

  // The history list (issue #60): the one implementation of browsing past translations,
  // mounted both by the history window (HistoryScreen.svelte) and the Settings history tab.
  // All list rules live here or in the pure utils/historyList.ts; the window shell owns only
  // what is window-specific (the Esc close). The backend already pages (GetHistory with
  // offset/limit); pages are appended as the user scrolls, so thousands of rows never render
  // at once.

  // The page size asked of the backend per load. 50 rows is well under a scroll's worth of
  // clamped rows.
  const PAGE_SIZE = 50;

  let keyword = $state('');
  let pairValue = $state('');
  let engineValue = $state('');
  let rows = $state<HistoryRow[]>([]);
  let total = $state(0);
  // The first load (or a reload after a filter change): the list is replaced, the states show.
  let loading = $state(false);
  // An appended page: the list stays, only the sentinel's label changes.
  let loadingMore = $state(false);
  // Rows whose texts the user unfolded ("show more"), by id.
  let expanded = $state<Set<number>>(new Set());
  // The keyboard-selected row (up/down/enter/delete), null when none.
  let selectedId = $state<number | null>(null);
  let searchEl = $state<HTMLInputElement | null>(null);
  let sentinelEl = $state<HTMLElement | null>(null);
  let listEl = $state<HTMLElement | null>(null);

  // "Now" for the day grouping and the time labels, read once per mount: a row's heading is
  // which day it belongs to, not a live clock.
  const mountNow = Date.now();

  const groups = $derived(groupDays(rows, mountNow));
  const pairOpts = $derived(pairOptions(rows));
  const engineOpts = $derived(engineOptions(rows));
  const pair = $derived(pairOpts.find((p) => p.value === pairValue) ?? null);
  // The states read "empty" and "nothing matches" differently (issue #60): a filter (keyword,
  // pair or engine) makes an empty result a miss, not a never-used list.
  const filtered = $derived(keyword.trim() !== '' || pairValue !== '' || engineValue !== '');

  let loadToken = 0;
  // One load of one page: append when offset > 0, replace otherwise. loadToken discards a
  // load that a newer reload replaced while it was in flight (a fast filter change).
  async function loadPage(append: boolean) {
    const kw = keyword.trim();
    const from = pair?.from ?? '';
    const to = pair?.to ?? '';
    const offset = append ? rows.length : 0;
    const token = ++loadToken;
    if (append) loadingMore = true;
    else loading = true;
    try {
      const n = await CountHistory(kw, from, to, engineValue);
      const page = (await GetHistory(kw, from, to, engineValue, offset, PAGE_SIZE)) ?? [];
      if (token !== loadToken) return;
      total = n;
      rows = append ? [...rows, ...page] : page;
      // A deleted row may have shifted the boundary: keep the selection only while the row
      // survives.
      if (selectedId !== null && !rows.some((r) => r.id === selectedId)) {
        selectedId = rows[0]?.id ?? null;
      }
    } catch (e) {
      // A load that a newer load or a local mutation already superseded does not own the
      // list any more: its failure belongs to a query nobody is waiting for, so it is
      // dropped like its result would be (PR #67 review).
      if (token !== loadToken) return;
      console.error(t('log.historyLoadFailed'), e);
      await Dialogs.Error({
        Title: t('history.opErrorTitle'),
        Message: t('history.loadError'),
      });
    } finally {
      if (token === loadToken) {
        loading = false;
        loadingMore = false;
      }
    }
  }

  // Invalidates every load still in flight (PR #67 review): a clear or a delete changes
  // rows/total locally, and a page append that left before the change would otherwise write
  // its stale rows and count back over the user's action when it lands. Advancing loadToken
  // makes loadPage's arrival checks drop it; the loading flags are reset here because a
  // dropped load's finally sees a newer token and deliberately leaves them alone, and no load
  // owns them any more. A reload (a filter or keyword change) already invalidates this way by
  // starting a fresh load of its own — this is the same rule for the mutations that change
  // the list without asking the backend for it.
  function invalidateLoads() {
    loadToken++;
    loading = false;
    loadingMore = false;
  }

  function reload() {
    void loadPage(false);
  }

  async function loadMore() {
    if (loading || loadingMore || !canLoadMore(rows.length, total)) return;
    await loadPage(true);
  }

  // Search input: debounce 300ms like the old Settings tab, then reload from the top.
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  function onSearch() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => reload(), 300);
  }

  function onFilterChange() {
    reload();
  }

  // Delete one entry: the row leaves the list locally (no refetch, so the scroll position and
  // the loaded pages stay), and the matching total drops. The local change invalidates every
  // in-flight load first (invalidateLoads), so a page append that left before the deletion
  // cannot write its stale rows back over the shortened list when it lands.
  async function deleteRow(id: number) {
    try {
      await DeleteHistory(id);
      invalidateLoads();
      rows = rows.filter((r) => r.id !== id);
      total = Math.max(0, total - 1);
      if (selectedId === id) selectedId = rows[0]?.id ?? null;
      const ex = new Set(expanded);
      ex.delete(id);
      expanded = ex;
    } catch (e) {
      console.error(t('log.historyDeleteFailed'), e);
      await Dialogs.Error({
        Title: t('history.opErrorTitle'),
        Message: t('history.deleteError'),
      });
    }
  }

  async function clearAll() {
    let answer = '';
    try {
      // The confirm is native (issue #60: "Clear all with a confirmation"): the question's
      // own buttons name the two outcomes; only the "clear" one proceeds.
      answer =
        (await Dialogs.Question({
          Title: t('history.clearConfirmTitle'),
          Message: t('history.clearConfirmMessage'),
          Buttons: [t('history.clearConfirmYes'), t('history.clearConfirmNo')],
        })) ?? '';
    } catch {
      return;
    }
    if (answer !== t('history.clearConfirmYes')) return;
    try {
      await ClearHistory();
      // The emptied list invalidates every in-flight load (invalidateLoads), so a page
      // append that left before the clear cannot repopulate the list when it lands.
      invalidateLoads();
      rows = [];
      total = 0;
      selectedId = null;
      expanded = new Set();
      await Dialogs.Info({ Title: t('history.clearedTitle'), Message: t('history.cleared') });
    } catch (e) {
      console.error(t('log.historyClearFailed'), e);
      await Dialogs.Error({
        Title: t('history.opErrorTitle'),
        Message: t('history.clearError'),
      });
    }
  }

  // "Use" (issue #60): broadcast the entry; the translate window restores its source, result,
  // languages and engine into the panes and translates nothing. The list stays open — the
  // user may bring back several entries in a row.
  function use(item: HistoryRow) {
    emitEvent(EventHistoryUse, usePayload(item));
  }

  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  function showToast(msg: string, durationMs = 1600) {
    toast = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = ''), durationMs);
  }

  async function copy(item: HistoryRow) {
    if (!item.result) return;
    try {
      await Clipboard.SetText(item.result);
      showToast(t('common.copied'));
    } catch (e) {
      console.error('copy failed', e);
    }
  }

  function toggleExpand(id: number) {
    const next = new Set(expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expanded = next;
  }

  function isExpanded(id: number): boolean {
    return expanded.has(id);
  }

  // The keyboard-selected row's element, for scroll-into-view. scrollIntoView may be absent
  // (jsdom); the optional call keeps the selection itself testable.
  function selectedEl(): HTMLElement | null {
    if (selectedId === null || !listEl) return null;
    return listEl.querySelector(`[data-id="${selectedId}"]`);
  }

  function selectRow(id: number, scroll = true) {
    selectedId = id;
    if (scroll) selectedEl()?.scrollIntoView?.({ block: 'nearest' });
  }

  function moveSelection(delta: number) {
    if (rows.length === 0) return;
    const idx = rows.findIndex((r) => r.id === selectedId);
    const next = idx === -1 ? 0 : Math.min(rows.length - 1, Math.max(0, idx + delta));
    selectRow(rows[next].id);
  }

  // The list's own keys (issue #60): up/down move, Enter uses, Delete removes, Cmd/Ctrl+F
  // focuses the search. Esc is NOT handled here — it closes the history window and belongs to
  // HistoryScreen. Keys pressed inside an editable control (the search box, the selects) are
  // that control's, not the list's.
  function onKeydown(e: KeyboardEvent) {
    const target = e.target;
    if (
      target instanceof HTMLElement &&
      (target.isContentEditable || ['TEXTAREA', 'INPUT', 'SELECT'].includes(target.tagName))
    ) {
      return;
    }
    const pressed = typeof e.key === 'string' ? e.key.toLowerCase() : '';
    if (pressed === 'f' && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) {
      e.preventDefault();
      searchEl?.focus();
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      moveSelection(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      moveSelection(-1);
    } else if (e.key === 'Enter') {
      const row = rows.find((r) => r.id === selectedId);
      if (row) {
        e.preventDefault();
        use(row);
      }
    } else if (e.key === 'Delete') {
      const row = rows.find((r) => r.id === selectedId);
      if (row) {
        e.preventDefault();
        void deleteRow(row.id);
      }
    }
  }

  onMount(() => {
    reload();
  });

  // Pages load as the sentinel scrolls into view (issue #60: "loaded in pages as the user
  // scrolls"). The sentinel exists only while more pages remain (the {#if canLoadMore(...)}
  // block around it), so the observer is armed by an effect that waits for it, never in
  // onMount: at mount the list is empty, canLoadMore is false, the sentinel is not rendered
  // and a one-shot registration silently never happens — which is exactly how the first
  // version lost auto-scroll pagination (PR #67 review). bind:this fills sentinelEl during
  // the DOM update and the effect runs after it. Reading rows.length and total re-arms the
  // observer for every appended page, so a sentinel that is STILL visible after a load keeps
  // loading (a fresh observer fires for already-intersecting elements; a reused one would
  // not, its intersection state never having changed) until the viewport fills. The effect's
  // cleanup disconnects whenever the sentinel leaves (the last page arrived, a filter change
  // emptied the list, or the component unmounted). The Load more button below the list stays
  // regardless: the fallback where IntersectionObserver is missing, and the explicit control.
  $effect(() => {
    if (!canLoadMore(rows.length, total)) return;
    const el = sentinelEl;
    if (el === null || typeof IntersectionObserver === 'undefined') return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((en) => en.isIntersecting)) void loadMore();
      },
      { root: listEl },
    );
    io.observe(el);
    return () => io.disconnect();
  });
</script>

<svelte:window onkeydown={onKeydown} />

<div class="flex h-full min-h-0 flex-col" style={rootStyleToStyle($rootStyle)}>
  <!-- Toolbar: search, the two filters, clear all (issue #60's header row). -->
  <div class="flex flex-wrap items-center gap-2 pb-3">
    <input
      bind:this={searchEl}
      class="u-field min-w-40 flex-1 px-3 py-2 text-sm"
      type="search"
      data-testid="history-search"
      placeholder={t('history.searchPlaceholder')}
      aria-label={t('history.searchPlaceholder')}
      bind:value={keyword}
      oninput={onSearch}
    />
    <select
      class="u-field u-select max-w-52 px-3 py-2 text-sm"
      data-testid="history-pair-filter"
      aria-label={t('history.allPairs')}
      bind:value={pairValue}
      onchange={onFilterChange}
    >
      <option value="">{t('history.allPairs')}</option>
      {#each pairOpts as p (p.value)}
        <option value={p.value}>{langName(p.from)} → {langName(p.to)}</option>
      {/each}
    </select>
    <select
      class="u-field u-select max-w-44 px-3 py-2 text-sm"
      data-testid="history-engine-filter"
      aria-label={t('history.allEngines')}
      bind:value={engineValue}
      onchange={onFilterChange}
    >
      <option value="">{t('history.allEngines')}</option>
      {#each engineOpts as en (en)}
        <option value={en}>{engineName(en)}</option>
      {/each}
    </select>
    {#if total > 0}
      <button
        class="u-btn u-btn--danger px-4 py-1.5 text-sm"
        data-testid="history-clear"
        onclick={clearAll}
      >
        {t('history.clearAll')}
      </button>
    {/if}
  </div>

  <div
    bind:this={listEl}
    class="u-card u-card--panel min-h-0 flex-1 overflow-y-auto p-0"
    role="listbox"
    aria-label={t('history.title')}
  >
    {#if loading}
      <div
        class="u-muted flex items-center justify-center py-16 text-sm"
        data-testid="history-loading"
      >
        {t('history.loading')}
      </div>
    {:else if rows.length === 0}
      {#if filtered}
        <div
          class="u-muted flex flex-col items-center justify-center gap-2 py-16 text-center text-sm"
          data-testid="history-no-match"
        >
          <p>{t('history.noMatch')}</p>
        </div>
      {:else}
        <div
          class="u-muted flex flex-col items-center justify-center gap-2 py-16 text-center text-sm"
          data-testid="history-empty"
        >
          <p>{t('history.empty')}</p>
        </div>
      {/if}
    {:else}
      {#each groups as g (g.key)}
        <section class="u-divide">
          <h2 class="u-muted u-border-b px-5 py-2 text-xs font-semibold uppercase tracking-wide">
            {g.kind === 'today'
              ? t('history.today')
              : g.kind === 'yesterday'
                ? t('history.yesterday')
                : g.date}
          </h2>
          <ul class="u-divide divide-y">
            {#each g.items as h (h.id)}
              <li
                data-testid="history-row"
                data-id={h.id}
                role="option"
                aria-selected={selectedId === h.id}
                class="u-card u-card--row flex cursor-pointer flex-col gap-2 px-5 py-3"
                class:u-history-selected={selectedId === h.id}
                onclick={() => selectRow(h.id, false)}
              >
                <!-- Meta line: time, the pair, the engine, the OCR mark (issue #60). -->
                <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                  <span class="u-muted tabular-nums" data-testid="history-row-time">
                    {rowTimeLabel(h.created_at, mountNow)}
                  </span>
                  <span>
                    {langName(h.from_lang)} → {langName(h.to_lang)}
                    {#if h.engine}· {engineName(h.engine)}{/if}
                    {#if h.from_ocr}
                      <span
                        class="u-muted u-history-ocr"
                        data-testid="history-row-ocr"
                        title={t('history.ocr')}
                      >
                        {t('history.ocr')}
                      </span>
                    {/if}
                  </span>
                  <!-- Row actions: Use, Copy, Delete. -->
                  <span class="ml-auto flex shrink-0 items-center gap-1">
                    <button
                      class="u-btn u-btn--ghost px-2.5 py-1 text-xs"
                      data-testid="history-use"
                      onclick={(e) => {
                        e.stopPropagation();
                        use(h);
                      }}
                    >
                      {t('history.use')}
                    </button>
                    <button
                      class="u-btn u-btn--ghost px-2.5 py-1 text-xs"
                      data-testid="history-copy"
                      disabled={!h.result}
                      onclick={(e) => {
                        e.stopPropagation();
                        void copy(h);
                      }}
                    >
                      {t('history.copy')}
                    </button>
                    <button
                      class="u-btn u-btn--ghost px-2.5 py-1 text-xs"
                      data-testid="history-delete"
                      onclick={(e) => {
                        e.stopPropagation();
                        void deleteRow(h.id);
                      }}
                    >
                      {t('common.delete')}
                    </button>
                  </span>
                </div>
                <!-- Source and result, each clamped to a few lines with a "show more"
                     (issue #60); break-words keeps a long unbroken string from widening the
                     row. -->
                <div class="min-w-0 space-y-1">
                  <p
                    class="break-words whitespace-pre-wrap text-sm leading-relaxed"
                    class:line-clamp-4={!isExpanded(h.id)}
                    data-testid="history-row-text"
                  >
                    {isExpanded(h.id) ? h.text : clampText(h.text)}
                  </p>
                  {#if h.result}
                    <p
                      class="u-text-accent break-words whitespace-pre-wrap text-sm leading-relaxed"
                      class:line-clamp-4={!isExpanded(h.id)}
                      data-testid="history-row-result"
                    >
                      {isExpanded(h.id) ? h.result : clampText(h.result)}
                    </p>
                  {/if}
                  {#if h.text.length > CLAMP_CHARS || h.result.length > CLAMP_CHARS}
                    <button
                      class="u-muted px-0 py-0.5 text-xs underline"
                      data-testid="history-show-more"
                      onclick={(e) => {
                        e.stopPropagation();
                        toggleExpand(h.id);
                      }}
                    >
                      {isExpanded(h.id) ? t('history.showLess') : t('history.showMore')}
                    </button>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
      <!-- The sentinel: reaching it loads the next page (the Load more button below is the
           always-there fallback and the explicit control). -->
      {#if canLoadMore(rows.length, total)}
        <div bind:this={sentinelEl} class="h-px w-full" aria-hidden="true"></div>
        <div class="flex items-center justify-center gap-2 p-3">
          <button
            class="u-btn u-btn--ghost px-4 py-1.5 text-sm"
            data-testid="history-load-more"
            disabled={loadingMore}
            onclick={() => void loadMore()}
          >
            {loadingMore ? t('history.loading') : t('history.loadMore')}
          </button>
        </div>
      {/if}
    {/if}
  </div>

  {#if toast}
    <div class="u-toast">{toast}</div>
  {/if}
</div>
