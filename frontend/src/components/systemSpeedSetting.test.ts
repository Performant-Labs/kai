import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #10: the copy tables this PR adds. The BEHAVIOUR (the notice rendering, the suggestion
// toast, the settings toggle, the progress line's time-left) is pinned by the mounted jsdom
// tests — systemSpeedMounted.test.ts and settings/enginesTabFastSystem.test.ts — which fail if
// the behaviour is removed; this file keeps only the data checks those tests cannot make (copy
// exists, differs between locales, carries its parameters), in the spirit of
// copyKeyFailedToast.test.ts's copy block.

describe('issue #10 copy exists and differs between locales', () => {
  it('timeLeft', () => {
    expect(en.translate.timeLeft).toBeTruthy();
    expect(zh.translate.timeLeft).toBeTruthy();
    expect(en.translate.timeLeft).not.toBe(zh.translate.timeLeft);
    expect(en.translate.timeLeft).toContain('{time}');
    expect(zh.translate.timeLeft).toContain('{time}');
  });
  it('fastSuggestion', () => {
    expect(en.translate.fastSuggestion).toBeTruthy();
    expect(zh.translate.fastSuggestion).toBeTruthy();
    expect(en.translate.fastSuggestion).not.toBe(zh.translate.fastSuggestion);
    expect(en.translate.fastSuggestion).toContain('{time}');
    expect(zh.translate.fastSuggestion).toContain('{time}');
  });
  it('settings labels and hints', () => {
    expect(en.settings.fastSystemTranslate).toBeTruthy();
    expect(zh.settings.fastSystemTranslate).toBeTruthy();
    expect(en.settings.fastSystemTranslateHint).toBeTruthy();
    expect(zh.settings.fastSystemTranslateHint).toBeTruthy();
  });

  // Reviewer's note 1: on macOS older than 26.4 the bridge's #available guard silently builds
  // the default (slow) session, so the fast mode is unavailable; the hint must say so.
  it('the hint says the fast mode needs macOS 26.4 or later, in both locales', () => {
    expect(en.settings.fastSystemTranslateHint).toContain('26.4');
    expect(zh.settings.fastSystemTranslateHint).toContain('26.4');
  });
});
