// Found by hand-testing #82. Two result-pane defects, both pre-existing, both pinned here at the
// pure-helper tier:
//  1. "Translation failed" flashed in the active engine's pane while a slower engine was still
//     working: any engine's first reply cleared `loading`, so the active engine (Apple, ~3 s) read
//     as failed while the fast one (Google, ~0.3 s) had answered. allReported() is the settle
//     rule: a request is only over when every enabled translate engine has reported (result or
//     failure), or the 15 s fallback fires.
//  2. The engine dropdown showed "Systemdisabled": the options come from GetEngines, which has no
//     `enabled` field, so every option read as disabled. isEngineOptionDisabled() decides from the
//     GetAllEngines list, and a suffix helper keeps a separator in front of the word.
import { describe, it, expect } from 'vitest';
import {
  allReported,
  isEngineOptionDisabled,
  engineOptionLabel,
  type PaneEngine,
  type PaneResult,
} from './resultPane.ts';

const eng = (value: string, over: Partial<PaneEngine> = {}): PaneEngine => ({
  id: 1,
  value,
  name: value,
  kind: 'translate',
  enabled: true,
  supported: true,
  ...over,
});

describe('allReported (the request has settled)', () => {
  const engines = [eng('apple'), eng('google')];
  it('is false while an enabled translate engine has not reported', () => {
    const results: Record<string, PaneResult> = { google: { engine: 'google', result: 'hi' } };
    expect(allReported(engines, results)).toBe(false);
  });
  it('is true once every enabled translate engine has an entry', () => {
    const results: Record<string, PaneResult> = {
      google: { engine: 'google', result: 'hi' },
      apple: { engine: 'apple', result: 'hello' },
    };
    expect(allReported(engines, results)).toBe(true);
  });
  it('counts a failure payload (no result text) as reported', () => {
    const results: Record<string, PaneResult> = {
      google: { engine: 'google', result: 'hi' },
      apple: { engine: 'apple', result: '', error: 'Unable to Translate' },
    };
    expect(allReported(engines, results)).toBe(true);
  });
  it('ignores disabled, unsupported and ocr engines', () => {
    const list = [
      eng('apple'),
      eng('google', { enabled: false }),
      eng('bing', { supported: false }),
      eng('vision', { kind: 'ocr' }),
    ];
    expect(allReported(list, { apple: { engine: 'apple', result: 'x' } })).toBe(true);
  });
  it('is false for nothing reported and true when there is nothing to wait for', () => {
    expect(allReported(engines, {})).toBe(false);
    expect(allReported([], {})).toBe(true);
  });
});

describe('engine dropdown option state', () => {
  const all = [eng('apple'), eng('google', { enabled: false })];
  it('an enabled engine is not disabled', () => {
    expect(isEngineOptionDisabled('apple', all)).toBe(false);
  });
  it('an engine switched off in settings is disabled', () => {
    expect(isEngineOptionDisabled('google', all)).toBe(true);
  });
  it('an engine unknown to the GetAllEngines list is not disabled (fail open, never all-disabled)', () => {
    expect(isEngineOptionDisabled('apple', [])).toBe(false);
    expect(isEngineOptionDisabled('mystery', all)).toBe(false);
  });
  it('the label puts a separator before the disabled word', () => {
    expect(engineOptionLabel('System', false, 'disabled')).toBe('System');
    expect(engineOptionLabel('System', true, 'disabled')).toBe('System (disabled)');
  });
});
