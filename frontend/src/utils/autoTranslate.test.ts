import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  AUTO_COMMIT_DELAY_MS,
  AUTO_MIN_TYPED_CHARS,
  AUTO_TYPING_DELAY_MS,
  autoKindOf,
  autoTranslateDelay,
  commitStillValid,
  createAutoScheduler,
} from './autoTranslate.ts';

// Issue #57: the pure decisions behind "translate as I type". When to translate (after a pause for
// typing, at once for a paste, never for a half-composed IME word or a programmatic fill, which
// translates on its own), the timer that starts it, and whether the history write that follows a
// quiet moment still describes the text on screen.

const base = { enabled: true, text: 'hola amigo', kind: 'typing' as const, hasEngines: true };

describe('autoKindOf', () => {
  it('reads the kind of an edit off its input type', () => {
    expect(autoKindOf('insertText', false)).toBe('typing');
    expect(autoKindOf('deleteContentBackward', false)).toBe('typing');
    expect(autoKindOf('insertLineBreak', false)).toBe('typing');
    expect(autoKindOf('insertFromPaste', false)).toBe('paste');
    expect(autoKindOf('insertFromDrop', false)).toBe('paste');
  });

  it('treats a cut or a drag-out as typing: the text that is left is retranslated after a pause', () => {
    expect(autoKindOf('deleteByCut', false)).toBe('typing');
    expect(autoKindOf('deleteByDrag', false)).toBe('typing');
  });

  it('marks everything an IME is doing as composing', () => {
    expect(autoKindOf('insertCompositionText', true)).toBe('compose');
    expect(autoKindOf('insertText', true)).toBe('compose');
    expect(autoKindOf('deleteCompositionText', false)).toBe('compose');
    expect(autoKindOf('insertFromComposition', false)).toBe('compose');
  });
});

describe('autoTranslateDelay', () => {
  it('waits for a pause after typing', () => {
    expect(autoTranslateDelay(base)).toBe(AUTO_TYPING_DELAY_MS);
    expect(AUTO_TYPING_DELAY_MS).toBeGreaterThanOrEqual(400);
    expect(AUTO_TYPING_DELAY_MS).toBeLessThanOrEqual(1000);
  });

  it('translates a paste at once', () => {
    expect(autoTranslateDelay({ ...base, kind: 'paste' })).toBe(0);
    expect(autoTranslateDelay({ ...base, kind: 'paste', text: 'a' })).toBe(0);
  });

  it('does nothing with the switch off, with no engine, or for blank text', () => {
    expect(autoTranslateDelay({ ...base, enabled: false })).toBeNull();
    expect(autoTranslateDelay({ ...base, hasEngines: false })).toBeNull();
    expect(autoTranslateDelay({ ...base, text: '   \n' })).toBeNull();
    expect(autoTranslateDelay({ ...base, kind: 'paste', text: '  ' })).toBeNull();
  });

  it('does not fire for one or two typed characters, but does at the minimum', () => {
    expect(autoTranslateDelay({ ...base, text: 'ho' })).toBeNull();
    expect(autoTranslateDelay({ ...base, text: ' ho ' })).toBeNull();
    expect(autoTranslateDelay({ ...base, text: 'x'.repeat(AUTO_MIN_TYPED_CHARS) })).toBe(
      AUTO_TYPING_DELAY_MS,
    );
    expect(autoTranslateDelay({ ...base, text: '你好吗' })).toBe(AUTO_TYPING_DELAY_MS);
  });

  it('never fires mid-composition, and leaves programmatic fills to their own path', () => {
    expect(autoTranslateDelay({ ...base, kind: 'compose' })).toBeNull();
    expect(autoTranslateDelay({ ...base, kind: 'program' })).toBeNull();
  });
});

describe('createAutoScheduler', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('runs once after the delay, and a new schedule restarts the wait', () => {
    const run = vi.fn();
    const s = createAutoScheduler(run);
    s.schedule(600);
    vi.advanceTimersByTime(500);
    s.schedule(600); // the user typed again
    vi.advanceTimersByTime(500);
    expect(run).not.toHaveBeenCalled();
    vi.advanceTimersByTime(100);
    expect(run).toHaveBeenCalledTimes(1);
    expect(s.pending()).toBe(false);
  });

  it('runs a zero delay at once, and cancels a wait that was pending', () => {
    const run = vi.fn();
    const s = createAutoScheduler(run);
    s.schedule(600);
    s.schedule(0);
    expect(run).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(2000);
    expect(run).toHaveBeenCalledTimes(1); // the earlier wait did not fire as well
  });

  it('cancel stops a pending run, and is harmless when nothing is pending', () => {
    const run = vi.fn();
    const s = createAutoScheduler(run);
    s.cancel();
    s.schedule(600);
    expect(s.pending()).toBe(true);
    s.cancel();
    expect(s.pending()).toBe(false);
    vi.advanceTimersByTime(2000);
    expect(run).not.toHaveBeenCalled();
  });
});

describe('commitStillValid', () => {
  it('holds only while the text on screen is the one the request was made for', () => {
    expect(
      commitStillValid({ requestText: 'hola amigo', currentText: 'hola amigo', current: true }),
    ).toBe(true);
    expect(
      commitStillValid({ requestText: 'hola amigo', currentText: 'hola amigos', current: true }),
    ).toBe(false);
    expect(
      commitStillValid({ requestText: 'hola amigo', currentText: 'hola amigo', current: false }),
    ).toBe(false);
    expect(commitStillValid({ requestText: '', currentText: '', current: true })).toBe(false);
  });

  it('waits long enough to be a quiet moment, longer than the typing pause', () => {
    expect(AUTO_COMMIT_DELAY_MS).toBeGreaterThan(AUTO_TYPING_DELAY_MS * 2);
  });
});
