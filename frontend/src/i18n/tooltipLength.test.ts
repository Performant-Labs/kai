import { describe, expect, it } from 'vitest';
import { en } from './en-US.ts';

// Issue #173, item 6: every tooltip site's English copy must be more descriptive than a bare
// 1-2 word label, while staying at or under 10 words (the issue's own cap). This pins the
// English source copy for the sites the issue named; zh-CN carries its own natural-language
// translations (word count is an English-specific proxy, not enforced there).
function wordCount(s: string): number {
  return s
    .trim()
    .split(/\s+/)
    .filter((w) => w.length > 0).length;
}

describe('expanded tooltip copy (issue #173 item 6)', () => {
  const sites: Array<[string, string]> = [
    ['common.copy', en.common.copy],
    ['translate.copy', en.translate.copy],
    ['translate.undo', en.translate.undo],
    ['translate.redo', en.translate.redo],
    ['translate.swap', en.translate.swap],
    ['translate.pin', en.translate.pin],
    ['translate.unpin', en.translate.unpin],
    ['translate.autoClipboard', en.translate.autoClipboard],
    ['translate.engineActive', en.translate.engineActive],
    ['titlebar.settingsHint', en.titlebar.settingsHint],
    ['screenshot.recaptureHint', en.screenshot.recaptureHint],
  ];

  it.each(sites)('%s is more than two words and at most ten words', (_key, text) => {
    const words = wordCount(text);
    expect(words).toBeGreaterThan(2);
    expect(words).toBeLessThanOrEqual(10);
  });
});
