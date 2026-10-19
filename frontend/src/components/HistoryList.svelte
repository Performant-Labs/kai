<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Clipboard, Dialogs } from '@wailsio/runtime';
  import { t, langName, engineName, locale } from '../i18n';
  import { emitEvent } from '../runtime';
  import { EventHistoryUse } from '../utils/events';
  import {
    QueryHistory,
    HistoryFacets,
    DeleteHistory,
    ClearHistory,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/historywrapper.ts';
  import { ShowTranslateWindow } from '@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts';
  import {
    PAGE_SIZE,
    appendPage,
    emptyFilter,
    emptyKind,
    emptyList,
    formatDayLabel,
    formatRowTime,
    groupByDay,
    hasMore,
    isFiltered,
    isLongText,
    moveSelection,
    removeRow,
    replacePage,
    shouldLoadMore,
    type HistoryFilter,
    type HistoryRow,
  } from '../utils/historyList';
  import { toUsePayload } from '../utils/historyUse';

  // The History list (issue #60), one component for the History window and the Settings tab, so
  // there is one list and one set of rules. The rules themselves (state, grouping, paging trigger,
  // selection) are pure functions in utils/historyList.ts; this is the wiring.
  let {
    title = '',
    onescape,
  }: {
    /** Heading shown left of the controls; the host decides (the window and the tab word it alike). */
    title?: string;
    /** Esc: the History window closes itself, the Settings tab has nothing to close. */
    onescape?: () => void;
  } = $props();

  type Facets = {
    pairs: { from_lang: string; to_lang: string }[];
    engines: { id: number; engine: string }[];
  };

  let filter = $state<HistoryFilter>(emptyFilter());
  let list = $state(emptyList());
  let facets = $state<Facets>({ pairs: [], engines: [] });
  let loading = $state(false);
  let failed = $state(false);
  let now = $state(Date.now());
  let selectedId = $state<number | null>(null);
  let expanded = $state<Record<number, boolean>>({});
  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  let scroller = $state<HTMLElement | null>(null);
  let searchEl = $state<HTMLInputElement | null>(null);

  // A response that arrives after a newer request was made (a keystroke, a filter) is dropped.
  let seq = 0;

  const groups = $derived(groupByDay(list.items, now));
  const empty = $derived(emptyKind(list, filter));
  const selectedIndex = $derived(list.items.findIndex((r) => r.id === selectedId));
  const uiLocale = $derived($locale);

  // The language-pair filter is one select: its value is "from|to".
  const pairValue = $derived(
    filter.from_lang === '' && filter.to_lang === '' ? '' : `${filter.from_lang}|${filter.to_lang}`,
  );

  function fail(message: string) {
    return Dialogs.Error({ Title: t('history.errorTitle'), Message: message }).catch(() => {});
  }

  async function fetchPage(offset: number, limit: number) {
    const page = await QueryHistory({ ...filter, keyword: filter.keyword.trim() }, offset, limit);
    return { items: (page?.items ?? []) as HistoryRow[], total: Number(page?.total ?? 0) };
  }

  // Loads the first page for the current filter (the list is replaced).
  async function reload() {
    const mine = ++seq;
    loading = true;
    failed = false;
    try {
      const page = await fetchPage(0, PAGE_SIZE);
      if (mine !== seq) return;
      now = Date.now();
      list = replacePage(page);
      if (selectedId !== null && !list.items.some((r) => r.id === selectedId)) selectedId = null;
    } catch (e) {
      if (mine !== seq) return;
      console.error(t('log.historyLoadFailed'), e);
      failed = true;
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Re-reads what is shown (the same rows, new entries included) without losing the scroll
  // position: the window is brought back to the front after a translation was saved.
  async function refresh() {
    if (loading) return;
    const mine = ++seq;
    try {
      const page = await fetchPage(0, Math.max(list.items.length, PAGE_SIZE));
      if (mine !== seq) return;
      now = Date.now();
      list = replacePage(page);
      if (selectedId !== null && !list.items.some((r) => r.id === selectedId)) selectedId = null;
    } catch (e) {
      console.error(t('log.historyLoadFailed'), e);
    }
    await loadFacets();
  }

  async function loadMore() {
    if (loading || !hasMore(list)) return;
    const mine = seq;
    loading = true;
    try {
      const page = await fetchPage(list.items.length, PAGE_SIZE);
      if (mine !== seq) return;
      list = appendPage(list, page);
    } catch (e) {
      if (mine !== seq) return;
      console.error(t('log.historyLoadFailed'), e);
      failed = true;
    } finally {
      if (mine === seq) loading = false;
    }
    await tick();
    maybeLoadMore();
  }

  function maybeLoadMore() {
    const el = scroller;
    if (!el) return;
    if (shouldLoadMore(el, { loading, hasMore: hasMore(list) })) loadMore();
  }

  async function loadFacets() {
    try {
      const f = await HistoryFacets();
      facets = { pairs: f?.pairs ?? [], engines: f?.engines ?? [] };
    } catch (e) {
      console.error(t('log.historyLoadFailed'), e);
    }
  }

  // The search box: debounced, then the list restarts from the top.
  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  function onSearch() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(reload, 300);
  }

  function setPair(value: string) {
    const [from, to] = value === '' ? ['', ''] : value.split('|');
    filter = { ...filter, from_lang: from, to_lang: to };
    reload();
  }
  function setEngine(value: string) {
    filter = { ...filter, engine_id: Number(value) || 0 };
    reload();
  }
  function clearFilters() {
    clearTimeout(searchTimer);
    filter = emptyFilter();
    reload();
  }

  function showToast(msg: string) {
    toast = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = ''), 1600);
  }

  // Use: the translate window gets the entry and comes to the front. Nothing is translated.
  async function useRow(row: HistoryRow) {
    try {
      await ShowTranslateWindow();
    } catch (e) {
      console.error(t('log.historyUseFailed'), e);
    }
    emitEvent(EventHistoryUse, toUsePayload(row));
  }

  async function copyRow(row: HistoryRow) {
    if (!row.result) return;
    try {
      await Clipboard.SetText(row.result);
      showToast(t('common.copied'));
    } catch (e) {
      console.error('copy failed', e);
    }
  }

  async function deleteRow(row: HistoryRow) {
    const index = list.items.findIndex((r) => r.id === row.id);
    try {
      await DeleteHistory(row.id);
    } catch (e) {
      console.error(t('log.historyDeleteFailed'), e);
      await fail(t('history.deleteError'));
      return;
    }
    list = removeRow(list, row.id);
    if (selectedId === row.id) {
      // The selection moves on to the row that took its place, so Delete can be pressed again.
      selectedId = list.items[Math.min(index, list.items.length - 1)]?.id ?? null;
    }
    await loadFacets();
    // A page that is now short may leave the viewport unfilled.
    await tick();
    maybeLoadMore();
  }

  async function clearAll() {
    const confirmLabel = t('history.clearAll');
    let answer = '';
    try {
      answer = await Dialogs.Question({
        Title: t('history.clearConfirmTitle'),
        Message: t('history.clearConfirmMessage'),
        Buttons: [
          { Label: t('common.cancel'), IsCancel: true, IsDefault: true },
          { Label: confirmLabel },
        ],
      });
    } catch (e) {
      console.error(t('log.historyClearFailed'), e);
      return;
    }
    if (answer !== confirmLabel) return;
    try {
      await ClearHistory();
    } catch (e) {
      console.error(t('log.historyClearFailed'), e);
      await fail(t('history.clearError'));
      return;
    }
    selectedId = null;
    filter = emptyFilter();
    clearTimeout(searchTimer);
    await Promise.all([reload(), loadFacets()]);
  }

  function select(row: HistoryRow) {
    selectedId = row.id;
  }

  function isTyping(target: EventTarget | null): boolean {
    return (
      target instanceof HTMLElement &&
      (target.isContentEditable || ['TEXTAREA', 'INPUT', 'SELECT'].includes(target.tagName))
    );
  }
  function isButton(target: EventTarget | null): boolean {
    return target instanceof HTMLElement && target.closest('button') !== null;
  }

  async function onKeydown(e: KeyboardEvent) {
    // While an IME composition is open the key belongs to the IME.
    if (e.isComposing) return;
    if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'f') {
      e.preventDefault();
      searchEl?.focus();
      searchEl?.select();
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'Escape') {
      if (onescape) {
        e.preventDefault();
        onescape();
      }
      return;
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      // A select keeps its own arrow keys (they change its value).
      if (e.target instanceof HTMLSelectElement) return;
      e.preventDefault();
      const next = moveSelection(selectedIndex, e.key, list.items.length);
      selectedId = list.items[next]?.id ?? null;
      await tick();
      scroller?.querySelector('[aria-current="true"]')?.scrollIntoView?.({ block: 'nearest' });
      return;
    }
    const row = selectedIndex >= 0 ? list.items[selectedIndex] : null;
    if (
      e.key === 'Enter' &&
      row &&
      !isButton(e.target) &&
      !(e.target instanceof HTMLSelectElement)
    ) {
      e.preventDefault();
      useRow(row);
      return;
    }
    // Delete in the search box deletes characters, not the entry.
    if (e.key === 'Delete' && row && !isTyping(e.target)) {
      e.preventDefault();
      deleteRow(row);
    }
  }

  onMount(() => {
    reload();
    loadFacets();
    return () => {
      clearTimeout(searchTimer);
      clearTimeout(toastTimer);
    };
  });

  function dayLabel(g: { kind: string; date: Date }): string {
    if (g.kind === 'today') return t('history.today');
    if (g.kind === 'yesterday') return t('history.yesterday');
    return formatDayLabel(g.date, now, uiLocale);
  }
</script>

<svelte:window onkeydown={onKeydown} onfocus={refresh} />

<div class="flex h-full min-h-0 flex-col gap-3">
  <div class="flex flex-wrap items-center gap-2">
    {#if title}
      <h1 class="mr-2 text-2xl font-semibold">{title}</h1>
    {/if}
    <input
      bind:this={searchEl}
      class="u-field min-w-48 flex-1 px-3 py-2 text-sm"
      type="search"
      placeholder={t('history.searchPlaceholder')}
      aria-label={t('history.searchPlaceholder')}
      bind:value={filter.keyword}
      oninput={onSearch}
    />
    <select
      class="u-field u-select px-3 py-2 text-sm"
      aria-label={t('history.filterPair')}
      value={pairValue}
      onchange={(e) => setPair(e.currentTarget.value)}
    >
      <option value="">{t('history.filterPair')}: {t('history.filterAny')}</option>
      {#each facets.pairs as p (`${p.from_lang}|${p.to_lang}`)}
        <option value={`${p.from_lang}|${p.to_lang}`}>
          {langName(p.from_lang)} → {langName(p.to_lang)}
        </option>
      {/each}
    </select>
    <select
      class="u-field u-select px-3 py-2 text-sm"
      aria-label={t('history.filterEngine')}
      value={String(filter.engine_id)}
      onchange={(e) => setEngine(e.currentTarget.value)}
    >
      <option value="0">{t('history.filterEngine')}: {t('history.filterAny')}</option>
      {#each facets.engines as e (e.id)}
        <option value={String(e.id)}>{engineName(e.engine)}</option>
      {/each}
    </select>
    <button
      class="u-btn u-btn--danger px-4 py-2 text-sm"
      disabled={empty === 'empty' || !list.loaded}
      onclick={clearAll}
    >
      {t('history.clearAll')}
    </button>
  </div>

  {#if list.loaded && list.items.length > 0}
    <p class="u-muted text-xs">{t('history.count', { count: list.total })}</p>
  {/if}

  <div
    bind:this={scroller}
    class="u-card u-card--panel min-h-0 flex-1 overflow-y-auto p-0"
    onscroll={maybeLoadMore}
    data-testid="history-scroller"
  >
    {#if empty === 'empty'}
      <div class="flex flex-col items-center justify-center gap-1 py-16 text-center">
        <p class="text-sm">{t('history.empty')}</p>
        <p class="u-muted text-xs">{t('history.emptyHint')}</p>
      </div>
    {:else if empty === 'nomatch'}
      <div class="flex flex-col items-center justify-center gap-2 py-16 text-center">
        <p class="text-sm">{t('history.noMatch')}</p>
        <p class="u-muted text-xs">{t('history.noMatchHint')}</p>
        <button class="u-btn u-btn--ghost px-3 py-1.5 text-sm" onclick={clearFilters}>
          {t('history.clearFilters')}
        </button>
      </div>
    {:else if failed && list.items.length === 0}
      <div class="flex flex-col items-center justify-center gap-2 py-16 text-center">
        <p class="text-sm">{t('history.loadError')}</p>
      </div>
    {:else}
      {#each groups as g (g.key)}
        <section>
          <h2 class="u-label u-border-b sticky top-0 z-10 px-5 py-1.5 text-xs">{dayLabel(g)}</h2>
          <ul class="u-divide divide-y">
            {#each g.rows as row (row.id)}
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
              <li
                class="u-card u-card--row flex flex-col gap-2 px-5 py-3"
                class:is-selected={row.id === selectedId}
                aria-current={row.id === selectedId ? 'true' : undefined}
                data-history-id={row.id}
                onclick={() => select(row)}
                ondblclick={() => useRow(row)}
              >
                <p class="u-muted text-xs">
                  <span>{formatRowTime(row.created_at, now, uiLocale)}</span>
                  <span>
                    · {langName(row.from_lang)} → {langName(row.to_lang)}{#if row.engine}
                      · {engineName(row.engine)}{/if}
                  </span>
                  {#if row.from_ocr}
                    <span class="rounded border px-1">{t('history.ocr')}</span>
                  {/if}
                </p>
                <p
                  class="break-words text-sm whitespace-pre-wrap"
                  class:line-clamp-3={isLongText(row.text) && !expanded[row.id]}
                >
                  {row.text}
                </p>
                {#if row.result}
                  <p
                    class="u-text-accent break-words text-sm whitespace-pre-wrap"
                    class:line-clamp-3={isLongText(row.result) && !expanded[row.id]}
                  >
                    {row.result}
                  </p>
                {/if}
                <div class="flex items-center justify-between gap-2">
                  <div>
                    {#if isLongText(row.text) || isLongText(row.result)}
                      <button
                        class="u-btn u-btn--ghost px-2 py-0.5 text-xs"
                        aria-expanded={Boolean(expanded[row.id])}
                        onclick={(e) => {
                          e.stopPropagation();
                          expanded = { ...expanded, [row.id]: !expanded[row.id] };
                        }}
                      >
                        {expanded[row.id] ? t('history.showLess') : t('history.showMore')}
                      </button>
                    {/if}
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <button
                      class="u-btn px-3 py-1 text-xs"
                      title={t('history.useHint')}
                      onclick={(e) => {
                        e.stopPropagation();
                        useRow(row);
                      }}>{t('history.use')}</button
                    >
                    <button
                      class="u-btn u-btn--ghost px-3 py-1 text-xs"
                      disabled={!row.result}
                      onclick={(e) => {
                        e.stopPropagation();
                        copyRow(row);
                      }}>{t('history.copy')}</button
                    >
                    <button
                      class="u-btn u-btn--ghost px-3 py-1 text-xs"
                      onclick={(e) => {
                        e.stopPropagation();
                        deleteRow(row);
                      }}>{t('history.delete')}</button
                    >
                  </div>
                </div>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
      {#if loading}
        <p class="u-muted py-3 text-center text-xs">{t('history.loading')}</p>
      {/if}
    {/if}
  </div>

  {#if toast}
    <div class="u-toast" role="status">{toast}</div>
  {/if}
</div>

<style>
  li.is-selected {
    outline: 2px solid var(--app-accent);
    outline-offset: -2px;
  }
</style>
