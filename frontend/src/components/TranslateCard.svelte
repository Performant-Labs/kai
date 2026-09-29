<script lang="ts">
  import { t, langName, engineName } from '../i18n';
  import { Clipboard } from '@wailsio/runtime';
  import { untrack } from 'svelte';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import { failureMessage } from '../utils/resultPane.ts';

  let {
    tr,
    expanded = true,
    onCopied,
  }: {
    tr: TranslateResult;
    expanded?: boolean;
    onCopied?: (text: string) => void;
  } = $props();

  // Why this engine failed (issue #96), when the backend sent a reason: the same failureMessage the
  // translate window's failed pane renders, so both windows read one copy table. An entry without
  // `error` keeps the fixed screenshot.translateFailed badge and shows no detail row. The card is
  // read-only, so the action failureMessage may return (a Settings button) is deliberately unused
  // here; the translate window is where a key gets fixed.
  const failure = $derived(
    tr && !tr.result && tr.error ? failureMessage(tr, t, engineName(tr.engine ?? '')) : null,
  );

  // The card is collapsible: by default only the first two successful translations start expanded
  // (the parent computes `expanded` and passes it in); the rest start collapsed.
  // expanded is only the initial value — clicking the header toggle changes isOpen itself, so the
  // prop never needs to be synced back; untrack reads the initial value explicitly, silencing
  // Svelte's state_referenced_locally warning.
  let isOpen = $state(untrack(() => expanded));

  async function copyText(text: string | undefined) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      onCopied?.(text ?? '');
    } catch (e) {
      console.error(t('log.translateCardCopyFailed'), e);
    }
  }
</script>

<div class="u-card p-3">
  <!-- This div dynamically acts as a button when a translation result exists (role=button +
       tabindex + keyboard support). It is an intentionally interactive element, so the
       non-interactive-element tabindex static check is ignored. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    class={tr?.result
      ? 'flex items-center justify-between'
      : 'flex items-start justify-between gap-2'}
    class:cursor-pointer={tr?.result}
    role={tr?.result ? 'button' : undefined}
    tabindex={tr?.result ? 0 : undefined}
    aria-expanded={tr?.result ? isOpen : undefined}
    aria-label={tr?.result
      ? isOpen
        ? t('screenshot.collapse')
        : t('screenshot.expand')
      : undefined}
    onclick={() => tr?.result && (isOpen = !isOpen)}
    onkeydown={(e) => {
      if (tr?.result && (e.key === 'Enter' || e.key === ' ')) {
        e.preventDefault();
        isOpen = !isOpen;
      }
    }}
  >
    <span class="text-xs font-semibold text-[var(--app-accent)]"
      >{engineName(tr?.engine ?? '')}</span
    >
    <!-- A failed card stacks the language line over the failure headline, right-aligned, so a long
         headline wraps in its own column instead of squeezing the language line. -->
    <div
      class={tr?.result ? 'flex items-center gap-2' : 'flex flex-col items-end gap-0.5 text-right'}
    >
      <span class="text-2xs text-[var(--app-muted)]">
        {t('screenshot.source')}: {langName(tr?.from ?? '')} → {langName(tr?.to ?? '')}
      </span>
      {#if tr?.cancelled && !tr?.result}
        <!-- The user cancelled this engine and it produced nothing (issue #109): a muted badge,
             never the failure one. -->
        <span class="text-2xs text-[var(--app-muted)]">{t('screenshot.cancelled')}</span>
      {:else if !tr?.result}
        <span class="text-2xs font-medium text-[var(--app-danger)]"
          >{failure ? failure.headline : t('screenshot.translateFailed')}</span
        >
      {:else}
        {#if tr?.cancelled}
          <!-- Cancelled with the parts already translated (the chunked translation, #84): they
               stay on the card, marked. -->
          <span class="text-2xs text-[var(--app-muted)]">{t('screenshot.cancelled')}</span>
        {/if}
        <svg
          class="h-3.5 w-3.5 text-[var(--app-muted)] transition-transform"
          class:rotate-180={isOpen}
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="m6 9 6 6 6-6"></path>
        </svg>
        <button
          class="u-btn u-btn--ghost p-1"
          title={t('common.copy')}
          aria-label={t('common.copy')}
          onclick={(e) => {
            e.stopPropagation();
            copyText(tr?.result);
          }}
        >
          <svg
            class="h-3.5 w-3.5"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
          </svg>
        </button>
      {/if}
    </div>
  </div>
  {#if failure?.detail}
    <!-- The sanitized reason under the header, muted; hidden when the error is empty. -->
    <p class="mt-1 break-words text-2xs text-[var(--app-muted)]" data-testid="failure-detail">
      {failure.detail}
    </p>
  {/if}
  {#if tr?.identity && tr?.result}
    <!-- Same language on both sides (issue #80): the result is the source text, not a translation;
         say so. Display only: it never changes the window's target select (its change would
         re-emit EventScreenshotRetranslate). -->
    <p class="mt-1 text-2xs text-[var(--app-muted)]" data-testid="identity-result">
      {t('translate.identity')}
    </p>
  {/if}
  {#if isOpen && tr?.result}
    <div class="mt-1 whitespace-pre-wrap break-words text-sm leading-relaxed">
      {tr.result}
    </div>
  {/if}
</div>
