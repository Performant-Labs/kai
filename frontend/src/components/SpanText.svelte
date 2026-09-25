<script lang="ts">
  // Pure display component (issue #11): renders text as hover-highlighted spans based on
  // tokenize's tokens.
  // Deliberately has no click behavior — word-level interaction (dictionary/alternatives)
  // belongs to issue #18.
  // Whitespace tokens render as plain text to preserve the original line-break/wrapping behavior.
  import { tokenize } from '../utils/tokenize.ts';

  let { text, class: cls = '' }: { text: string; class?: string } = $props();

  const tokens = $derived(tokenize(text));
</script>

{#each tokens as t (t.start)}
  {#if t.kind === 'space'}{t.text}{:else}<span
      class="rounded px-0.5 hover:bg-[var(--app-accent)]/20 {cls}">{t.text}</span
    >{/if}
{/each}
