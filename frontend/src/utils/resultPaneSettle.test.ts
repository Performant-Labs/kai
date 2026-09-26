// Found by hand-testing #82. Two result-pane defects, both pre-existing, both pinned here at the
// pure-helper tier:
//  1. "Translation failed" flashed in the active engine's pane while a slower engine was still
//     working: any engine's first reply cleared `loading`, so the active engine (Apple, ~3 s) read
//     as failed while the fast one (Google, ~0.3 s) had answered. allReported() is the settle
//     rule: a request is only over when every started engine has reported (result, failure or
//     cancel). #109 removed the timer fallback.
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

// #109: the settle rule reads what the backend STARTED (the `engines` list TranslateMulti returns),
// not the enabled-engine list, and there is no timer fallback (see components/noTimeLimit.test.ts).
describe('allReported (every started engine has reported)', () => {
  const started = ['apple', 'google'];
  it('is false while a started engine has not reported', () => {
    const results: Record<string, PaneResult> = { google: { engine: 'google', result: 'hi' } };
    expect(allReported(started, results)).toBe(false);
  });
  it('is true once every started engine has an entry', () => {
    const results: Record<string, PaneResult> = {
      google: { engine: 'google', result: 'hi' },
      apple: { engine: 'apple', result: 'hello' },
    };
    expect(allReported(started, results)).toBe(true);
  });
  it('counts a failure payload (no result text) and a cancelled payload as reported', () => {
    const results: Record<string, PaneResult> = {
      google: { engine: 'google', result: '', cancelled: true },
      apple: { engine: 'apple', result: '', error: 'Unable to Translate' },
    };
    expect(allReported(started, results)).toBe(true);
  });
  it('an enabled engine the backend did not start does not block (only the started list counts)', () => {
    // bing is enabled in the engine list but was not started, so it is not in `started`.
    expect(allReported(['apple'], { apple: { engine: 'apple', result: 'x' } })).toBe(true);
  });
  it('is false for nothing reported and true when there is nothing to wait for', () => {
    expect(allReported(started, {})).toBe(false);
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
