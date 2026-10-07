import { describe, expect, it } from 'vitest';
import {
  addUserMessage,
  applyAnswer,
  answerNoteKey,
  buildContextRequest,
  clearChat,
  emptyChat,
  hasContext,
  isClearShortcut,
  validateClearShortcut,
  DEFAULT_CLEAR_SHORTCUT,
  shownFor,
  type ContextAnswer,
} from './contextChat.ts';

// Issue #48: the pure state of the result pane's chat. The decisions (which engine answers, the
// fallback order, the prompt) are the backend's, tested in Go; this keeps the context across
// texts until it is cleared, builds the request, and says what an answer means for the window.

const answer = (over: Partial<ContextAnswer> = {}): ContextAnswer => ({
  status: 'ok',
  result: 'I need tomorrow’s meeting',
  engine: 'openai',
  fallback: false,
  reason: '',
  error: '',
  request_id: 'r1',
  ...over,
});

describe('the context', () => {
  it('starts empty and has no context', () => {
    expect(hasContext(emptyChat)).toBe(false);
  });

  it('collects the user messages, oldest first, trimmed; a blank one is ignored', () => {
    let c = addUserMessage(emptyChat, '  junta means a meeting ');
    c = addUserMessage(c, 'be formal');
    c = addUserMessage(c, '   ');
    expect(c.context).toEqual(['junta means a meeting', 'be formal']);
    expect(c.messages.map((m) => m.role)).toEqual(['user', 'user']);
    expect(hasContext(c)).toBe(true);
  });

  it('is kept when the text changes: nothing here is keyed to a text', () => {
    const c = addUserMessage(emptyChat, 'be formal');
    const r1 = buildContextRequest(c, {
      text: 'uno',
      from: 'es-MX',
      to: 'en',
      engine: 'apple',
      previous: 'one',
      requestId: 'a',
    });
    const r2 = buildContextRequest(c, {
      text: 'dos',
      from: 'es-MX',
      to: 'en',
      engine: 'apple',
      previous: '',
      requestId: 'b',
    });
    expect(r1.context).toEqual(['be formal']);
    expect(r2.context).toEqual(['be formal']);
  });

  it('is removed, with the chat history, by clearChat', () => {
    const c = applyAnswer(addUserMessage(emptyChat, 'be formal'), answer()).chat;
    expect(c.messages.length).toBeGreaterThan(1);
    const cleared = clearChat();
    expect(cleared).toEqual(emptyChat);
    expect(hasContext(cleared)).toBe(false);
    expect(
      buildContextRequest(cleared, {
        text: 'x',
        from: 'es',
        to: 'en',
        engine: 'apple',
        previous: '',
        requestId: 'c',
      }).context,
    ).toEqual([]);
  });
});

describe('buildContextRequest', () => {
  it('carries the text, the pair, the engine, the previous translation and every message', () => {
    const c = addUserMessage(addUserMessage(emptyChat, 'legal contract'), 'shorter');
    expect(
      buildContextRequest(c, {
        text: 'hola',
        from: 'es-MX',
        to: 'en',
        engine: 'apple',
        previous: 'hello',
        requestId: 'q',
      }),
    ).toEqual({
      text: 'hola',
      from: 'es-MX',
      to: 'en',
      engine: 'apple',
      previous: 'hello',
      context: ['legal contract', 'shorter'],
      request_id: 'q',
    });
  });
});

describe('applyAnswer', () => {
  it('shows an ok answer, labelled with the engine that produced it, and notes a fallback', () => {
    const base = addUserMessage(emptyChat, 'be formal');
    const direct = applyAnswer(base, answer());
    expect(direct.shown).toEqual({
      text: 'I need tomorrow’s meeting',
      engine: 'openai',
      fallback: false,
    });
    expect(direct.chat.messages.at(-1)).toMatchObject({
      role: 'assistant',
      text: 'I need tomorrow’s meeting',
    });

    const fb = applyAnswer(base, answer({ engine: 'apple-foundation-models', fallback: true }));
    expect(fb.shown).toEqual({
      text: 'I need tomorrow’s meeting',
      engine: 'apple-foundation-models',
      fallback: true,
    });
  });

  it('shows nothing for any other status, and says why in the chat', () => {
    const base = addUserMessage(emptyChat, 'be formal');
    for (const status of ['unavailable', 'failed', 'cancelled', 'no_context'] as const) {
      const r = applyAnswer(base, answer({ status, result: '' }));
      expect(r.shown, status).toBeNull();
      expect(r.chat.context, status).toEqual(['be formal']);
    }
    // an ok answer with no text is not an answer
    expect(applyAnswer(base, answer({ result: '  ' })).shown).toBeNull();
  });
});

describe('answerNoteKey', () => {
  it('names the sentence that words each outcome', () => {
    expect(answerNoteKey(answer())).toBeNull();
    expect(answerNoteKey(answer({ status: 'unavailable', reason: 'apple_intelligence_off' }))).toBe(
      'translate.contextUnavailableAppleIntelligence',
    );
    expect(answerNoteKey(answer({ status: 'unavailable', reason: 'model_not_ready' }))).toBe(
      'translate.contextUnavailableNotReady',
    );
    expect(answerNoteKey(answer({ status: 'unavailable', reason: 'none_available' }))).toBe(
      'translate.contextUnavailable',
    );
    expect(answerNoteKey(answer({ status: 'failed' }))).toBe('translate.contextFailed');
    expect(answerNoteKey(answer({ status: 'cancelled' }))).toBeNull();
    expect(answerNoteKey(answer({ status: 'no_context' }))).toBeNull();
  });
});

describe('shownFor', () => {
  const shown = {
    text: 'x',
    engine: 'openai',
    fallback: false,
    forInput: 'hola',
    forEngine: 'apple',
  };
  it('applies only to the text and the engine it was made for', () => {
    expect(shownFor(shown, 'hola', 'apple')).toBe(shown);
    expect(shownFor(shown, 'hola!', 'apple')).toBeNull();
    expect(shownFor(shown, 'hola', 'google')).toBeNull();
    expect(shownFor(null, 'hola', 'apple')).toBeNull();
  });
});

describe('isClearShortcut', () => {
  const ev = (over: Partial<KeyboardEvent>) =>
    ({
      key: 'k',
      metaKey: true,
      shiftKey: true,
      ctrlKey: false,
      altKey: false,
      ...over,
    }) as KeyboardEvent;
  it('matches Cmd+Shift+K by default, exactly', () => {
    expect(isClearShortcut(ev({}))).toBe(true);
    expect(isClearShortcut(ev({ key: 'K' }))).toBe(true);
    expect(isClearShortcut(ev({ shiftKey: false }))).toBe(false);
    expect(isClearShortcut(ev({ metaKey: false }))).toBe(false);
    expect(isClearShortcut(ev({ altKey: true }))).toBe(false);
    expect(isClearShortcut(ev({ key: 'j' }))).toBe(false);
  });
  it('follows a configured combo', () => {
    expect(
      isClearShortcut(ev({ key: 'x', shiftKey: false, ctrlKey: true, metaKey: false }), 'Ctrl+X'),
    ).toBe(true);
    expect(isClearShortcut(ev({}), 'Ctrl+X')).toBe(false);
    expect(isClearShortcut(ev({}), '')).toBe(false);
  });
});

describe('isClearShortcut safety', () => {
  const ev = (over: Partial<KeyboardEvent>) =>
    ({
      key: 'k',
      metaKey: false,
      shiftKey: false,
      ctrlKey: false,
      altKey: false,
      ...over,
    }) as KeyboardEvent;
  it('never matches a combo without Cmd, Ctrl or Alt: it would fire while the user types', () => {
    expect(isClearShortcut(ev({}), 'K')).toBe(false);
    expect(isClearShortcut(ev({ shiftKey: true }), 'Shift+K')).toBe(false);
  });
});

describe('validateClearShortcut', () => {
  it('accepts a combo with a modifier and a key, written in the canonical order', () => {
    expect(validateClearShortcut('cmd+shift+k')).toEqual({ ok: true, value: 'Cmd+Shift+K' });
    expect(validateClearShortcut('Shift+Cmd+K')).toEqual({ ok: true, value: 'Cmd+Shift+K' });
    expect(validateClearShortcut(' Ctrl + Alt + x ')).toEqual({ ok: true, value: 'Alt+Ctrl+X' });
    expect(validateClearShortcut(DEFAULT_CLEAR_SHORTCUT)).toEqual({
      ok: true,
      value: DEFAULT_CLEAR_SHORTCUT,
    });
  });
  it('treats an empty value as "no shortcut"', () => {
    expect(validateClearShortcut('')).toEqual({ ok: true, value: '' });
    expect(validateClearShortcut('   ')).toEqual({ ok: true, value: '' });
  });
  it('refuses a combo with no modifier, or only Shift, or no key', () => {
    expect(validateClearShortcut('K')).toEqual({ ok: false, reason: 'needs_modifier' });
    expect(validateClearShortcut('Shift+K')).toEqual({ ok: false, reason: 'needs_modifier' });
    expect(validateClearShortcut('Cmd+Shift')).toEqual({ ok: false, reason: 'no_key' });
    expect(validateClearShortcut('Cmd+')).toEqual({ ok: false, reason: 'no_key' });
  });
  it("refuses the keys the window already uses (Cmd+Enter translates, Cmd+Z undoes) and the system's editing keys", () => {
    for (const c of [
      'Cmd+Enter',
      'Cmd+Z',
      'Cmd+Shift+Z',
      'Ctrl+Z',
      'Ctrl+Y',
      'Cmd+C',
      'Cmd+V',
      'Cmd+X',
      'Cmd+A',
      'Cmd+Q',
      'Cmd+W',
    ]) {
      expect(validateClearShortcut(c), c).toEqual({ ok: false, reason: 'reserved' });
    }
  });
});
