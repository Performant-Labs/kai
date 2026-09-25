<script lang="ts">
  // 纯展示组件（issue #11）：把文本按 tokenize 的 token 渲染成 hover 高亮的 span。
  // 刻意没有任何点击行为——词级交互（字典/备选）属于 issue #18。
  // 空白 token 以纯文本渲染，保持原有换行/折行行为。
  import { tokenize } from '../utils/tokenize.ts';

  let { text, class: cls = '' }: { text: string; class?: string } = $props();

  const tokens = $derived(tokenize(text));
</script>

{#each tokens as t (t.start)}
  {#if t.kind === 'space'}{t.text}{:else}<span
      class="rounded px-0.5 hover:bg-[var(--app-accent)]/20 {cls}">{t.text}</span
    >{/if}
{/each}
