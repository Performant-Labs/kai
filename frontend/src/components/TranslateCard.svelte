<script lang="ts">
  import { t, langName, engineName } from '../i18n';
  import { Clipboard } from '@wailsio/runtime';
  import { untrack } from 'svelte';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';

  let {
    tr,
    expanded = true,
    onCopied,
  }: { tr: TranslateResult; expanded?: boolean; onCopied?: (text: string) => void } = $props();

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
    class="flex items-center justify-between"
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
    <div class="flex items-center gap-2">
      <span class="text-[11px] text-[var(--app-muted)]">
        {t('screenshot.source')}: {langName(tr?.from ?? '')} → {langName(tr?.to ?? '')}
      </span>
      {#if !tr?.result}
        <span class="text-[11px] font-medium text-[var(--app-danger)]"
          >{t('screenshot.translateFailed')}</span
        >
      {:else}
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
  {#if isOpen && tr?.result}
    <div class="mt-1 whitespace-pre-wrap break-words text-sm leading-relaxed">
      {tr.result}
    </div>
  {/if}
</div>
