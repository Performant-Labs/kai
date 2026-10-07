<script lang="ts">
  import { t } from '../i18n';
  import type { ContextChat } from '../utils/contextChat.ts';

  // The result pane's context chat (issue #48): the user tells the translator it got the context
  // wrong, and the text is translated again in that context. This component only shows the chat and
  // reports what the user did; TranslateWindow owns the state and talks to the backend, which
  // decides which engine answers.
  let {
    chat,
    open,
    busy,
    active,
    shortcutLabel,
    ontoggle,
    onsend,
    onclear,
    oncancel,
    note,
    canShowOriginal,
    onshoworiginal,
  }: {
    chat: ContextChat;
    open: boolean;
    busy: boolean;
    /** A context is in force: it applies to every translation until it is cleared. */
    active: boolean;
    shortcutLabel: string;
    ontoggle: () => void;
    onsend: (message: string) => void;
    onclear: () => void;
    oncancel: () => void;
    /** Says which engine translated, when it is not the selected one. */
    note: string;
    /** A retranslation is on screen in place of the engine's own translation. */
    canShowOriginal: boolean;
    onshoworiginal: () => void;
  } = $props();

  let draft = $state('');

  function send() {
    const message = draft.trim();
    if (message === '' || busy) return;
    draft = '';
    onsend(message);
  }

  function onkeydown(e: KeyboardEvent) {
    // Enter sends; Shift+Enter is a new line. Cmd+Enter belongs to the window's Translate shortcut.
    if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.isComposing) {
      e.preventDefault();
      send();
    }
  }
</script>

<div class="u-border-t flex shrink-0 flex-col" data-testid="context-chat">
  <div class="flex items-center justify-between gap-2 px-3 py-2">
    <button
      class="u-btn u-btn--ghost u-no-drag px-2 py-1 text-xs"
      data-testid="context-toggle"
      aria-expanded={open}
      title={t('translate.contextOpen')}
      onclick={ontoggle}
    >
      {t('translate.contextOpen')}
    </button>
    {#if active}
      <span class="flex items-center gap-2">
        <span
          class="u-muted text-2xs"
          data-testid="context-active"
          title={t('translate.contextActiveHint')}
        >
          ● {t('translate.contextActive')}
        </span>
        <button
          class="u-btn u-btn--ghost u-no-drag px-2 py-1 text-xs"
          data-testid="context-clear"
          title={t('translate.contextClearHint', { shortcut: shortcutLabel })}
          aria-label={t('translate.contextClearHint', { shortcut: shortcutLabel })}
          onclick={onclear}
        >
          {t('translate.contextClear')}
        </button>
      </span>
    {/if}
  </div>
  {#if note || canShowOriginal}
    <div
      class="u-muted flex flex-wrap items-center gap-2 px-3 pb-2 text-2xs"
      data-testid="context-result-note"
    >
      {#if note}<span>{note}</span>{/if}
      {#if canShowOriginal}
        <button
          class="u-btn u-btn--ghost u-no-drag px-2 py-0.5 text-2xs"
          data-testid="context-show-original"
          onclick={onshoworiginal}
        >
          {t('translate.contextShowOriginal')}
        </button>
      {/if}
    </div>
  {/if}
  {#if open}
    <div class="flex max-h-56 min-h-0 flex-col gap-2 px-3 pb-3">
      <ul
        class="m-0 flex min-h-0 list-none flex-col gap-1 overflow-y-auto p-0"
        data-testid="context-messages"
      >
        {#each chat.messages as m}
          <li
            class="text-xs"
            class:u-muted={m.role !== 'user'}
            class:font-medium={m.role === 'user'}
            data-role={m.role}
          >
            {m.text}
          </li>
        {/each}
      </ul>
      {#if busy}
        <div class="flex items-center gap-2">
          <span class="u-muted text-xs">{t('translate.contextWorking')}</span>
          <button
            class="u-btn u-btn--ghost u-no-drag px-2 py-1 text-xs"
            data-testid="context-cancel"
            onclick={oncancel}
          >
            <span aria-hidden="true">✕</span>
            {t('translate.contextCancel')}
          </button>
        </div>
      {/if}
      <div class="flex items-end gap-2">
        <textarea
          class="u-field min-h-0 flex-1 resize-none px-2 py-1 text-sm"
          rows="2"
          data-testid="context-input"
          placeholder={t('translate.contextPlaceholder')}
          bind:value={draft}
          {onkeydown}></textarea>
        <button
          class="u-btn u-no-drag px-3 py-1 text-xs"
          data-testid="context-send"
          title={t('translate.contextSend')}
          disabled={busy || draft.trim() === ''}
          onclick={send}
        >
          {t('translate.contextSend')}
        </button>
      </div>
    </div>
  {/if}
</div>
