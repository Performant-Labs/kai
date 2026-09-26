import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #96: both windows and the failed-dot render failures through the one `failureMessage`
// helper, and the frontend reads the generated snake_case field. The repo has no Svelte render
// harness, so this pins the markup contract from the component sources (visual layout is the
// principal's hand test after build, per the approved wireframe).

const win = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const card = readFileSync(resolve(__dirname, 'TranslateCard.svelte'), 'utf8');

// The {:else if pane === 'failed'} branch, ending at the sibling {:else ...} or the closing {/if}
// of the same {#if} chain (nested {#if} blocks inside the branch are skipped by depth counting).
function failedBranch(): string {
  const marker = "{:else if pane === 'failed'}";
  const start = win.indexOf(marker);
  if (start < 0) return '';
  let depth = 0;
  const re = /\{#if\b|\{\/if\}|\{:else/g;
  re.lastIndex = start + marker.length;
  for (let m = re.exec(win); m; m = re.exec(win)) {
    if (m[0] === '{#if') depth++;
    else if (m[0] === '{/if}') {
      if (depth === 0) return win.slice(start, m.index);
      depth--;
    } else if (depth === 0) return win.slice(start, m.index);
  }
  return win.slice(start);
}

describe('translate window failed pane', () => {
  const branch = failedBranch();

  it('renders the failureMessage result, not the literal generic key', () => {
    expect(branch, "a pane === 'failed' branch").not.toBe('');
    // The helper is actually called (the pre-#96 import was dead), and the branch renders its
    // headline and detail rather than t('translate.failed').
    expect(win.replace(/import[\s\S]*?from\s*'[^']*';/g, '')).toMatch(/failureMessage\(/);
    expect(branch).toMatch(/\.headline/);
    expect(branch).toMatch(/\.detail/);
    expect(branch).not.toMatch(/t\('translate\.failed'\)/);
  });

  it('shows the headline in --app-danger and the detail muted', () => {
    expect(branch).toMatch(/--app-danger/);
    expect(branch).toMatch(/muted/);
  });

  it("has a Settings button gated on action === 'settings' that calls ShowSettings()", () => {
    expect(branch).toMatch(/action\s*===\s*'settings'/);
    expect(branch).toMatch(/<button[\s\S]*?onclick=\{[^}]*ShowSettings\([^}]*\}[\s\S]*?<\/button>/);
    expect(branch).toMatch(/t\('titlebar\.settings'\)/);
  });
});

describe('failed status dot', () => {
  it('title and aria-label are built from failureMessage, falling back to engineFailed', () => {
    expect(win).toMatch(/title=\{[\s\S]{0,900}?(failureMessage\(|\.headline)/);
    expect(win).toMatch(/aria-label=\{[\s\S]{0,900}?(failureMessage\(|\.headline)/);
    expect(win).toMatch(/translate\.engineFailed/);
  });
});

describe('screenshot TranslateCard', () => {
  it('shows failureMessage output when tr.error is set and keeps the fixed badge otherwise', () => {
    expect(card).toMatch(/failureMessage\(/);
    expect(card).toMatch(/\.error\b/);
    expect(card).toMatch(/screenshot\.translateFailed/);
  });

  it('never offers a Settings button', () => {
    expect(card).not.toMatch(/ShowSettings/);
  });
});

describe('wire field name', () => {
  // Built from pieces so this file does not contain the retired name itself.
  const retired = new RegExp('error' + 'Kind');
  function walk(dir: string, out: string[] = []): string[] {
    for (const name of readdirSync(dir)) {
      const p = join(dir, name);
      if (statSync(p).isDirectory()) walk(p, out);
      else if (/\.(ts|svelte)$/.test(name) && !/\.test\.ts$/.test(name)) out.push(p);
    }
    return out;
  }
  it('no frontend production source reads the camelCase name (AC10)', () => {
    const offenders = walk(resolve(__dirname, '..')).filter((f) => retired.test(readFileSync(f, 'utf8')));
    expect(offenders).toEqual([]);
  });
});

describe('failure copy (AC14)', () => {
  const newKeys = [
    'failedNotConfigured',
    'failedQuota',
    'failedRateLimit',
    'failedUnavailable',
    'failedTooLong',
    'failedUnsupported',
  ];
  const enT = en.translate as Record<string, string>;
  const zhT = zh.translate as Record<string, string>;

  it('new keys exist in en-US and zh-CN, non-empty, and take {engine}', () => {
    for (const k of newKeys) {
      expect(enT[k], `en ${k}`).toBeTruthy();
      expect(zhT[k], `zh ${k}`).toBeTruthy();
      expect(enT[k], `en ${k} {engine}`).toContain('{engine}');
      expect(zhT[k], `zh ${k} {engine}`).toContain('{engine}');
    }
  });

  it('failedAuth and failedNetwork now take {engine}; failedPair stays apple-only (no {engine})', () => {
    for (const k of ['failedAuth', 'failedNetwork']) {
      expect(enT[k]).toContain('{engine}');
      expect(zhT[k]).toContain('{engine}');
    }
    expect(enT.failedPair).not.toContain('{engine}');
    expect(zhT.failedPair).not.toContain('{engine}');
  });
});
