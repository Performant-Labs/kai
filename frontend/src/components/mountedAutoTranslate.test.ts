import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #57: "Translate as I type", with the REAL TranslateWindow.svelte mounted in jsdom with real
// localStorage and fake timers. Only the Wails boundary is faked: the runtime (events, window) and the
// generated bindings. What the backend does with the request (which engines, the history rule) is
// tested in Go; this pins when the window asks, and what it asks.

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(e: { data: unknown }) => void>>();
  return {
    handlers,
    fire(name: string, data: unknown) {
      for (const cb of handlers.get(name) ?? []) cb({ data });
    },
    plan: vi.fn(),
    reportSkip: vi.fn(async (_reason: string) => {}),
    translate: vi.fn(),
    commit: vi.fn(),
    learn: vi.fn(),
    saveConfig: vi.fn(),
    config: { default_from: 'en', default_to: 'fr', auto_clipboard: false } as Record<
      string,
      unknown
    >,
  };
});

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, cb: (e: { data: unknown }) => void) => {
      h.handlers.set(name, [...(h.handlers.get(name) ?? []), cb]);
      return () =>
        h.handlers.set(
          name,
          (h.handlers.get(name) ?? []).filter((x) => x !== cb),
        );
    },
    Emit: vi.fn(),
  },
  Window: { SetAlwaysOnTop: vi.fn(async () => {}), Name: async () => 'translate' },
  Clipboard: { SetText: vi.fn(async () => {}) },
  System: { IsDarkMode: async () => false },
}));

vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts', () => ({
  TranslateMulti: (req: unknown) => h.translate(req),
  CancelTranslate: vi.fn(async () => true),
  PlanSourceSwitch: (req: unknown) => h.plan(req),
  ReportSourceSwitchSkipped: (reason: string) => h.reportSkip(reason),
  CorrectSource: vi.fn(),
  CommitAutoHistory: (id: string) => h.commit(id),
  CorrectionAvailability: async () => ({ available: true, reason: '' }),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts', () => ({
  Learn: (l: string) => h.learn(l),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts', () => ({
  ShowSettings: vi.fn(),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts', () => ({
  GetEngines: async () => [{ value: 'google', name: 'Google', kind: 'translate', supported: true }],
  GetAllEngines: async () => [
    {
      id: 0,
      value: 'google',
      name: 'Google',
      kind: 'translate',
      enabled: true,
      supported: true,
      target_languages: ['en', 'fr', 'es-MX'],
    },
  ],
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetLanguages: async () => ['auto', 'en', 'fr', 'es-MX'].map((v) => ({ value: v, name: v })),
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: (c: unknown) => h.saveConfig(c),
  GetTheme: async () => 'light',
  SetTheme: async () => {},
}));

import TranslateWindow from './TranslateWindow.svelte';
import { autoTranslateOn } from '../stores/autoTranslate';

const TEXT = 'Hola amigo mío';
let app: Record<string, unknown> | undefined;
let target: HTMLElement;

// Fake timers for the pause; microtasks are flushed by advancing by zero.
const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const advance = async (ms: number) => {
  await vi.advanceTimersByTimeAsync(ms);
  flushSync();
};
const source = () => target.querySelector('textarea') as HTMLTextAreaElement;
const type = (text: string, inputType = 'insertText') => {
  source().value = text;
  source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType }));
};
const reqOf = (n: number) =>
  h.translate.mock.calls[n][0] as { request_id: string; text: string; auto?: boolean };
const result = (id: string) =>
  h.fire('kai:translate:result', {
    engine: 'google',
    request_id: id,
    result: 'Hello',
    from: 'en',
    to: 'fr',
  });

beforeEach(async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  localStorage.clear();
  h.handlers.clear();
  h.plan.mockReset();
  h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
  h.translate.mockReset();
  h.translate.mockImplementation(async (req: { request_id: string }) => ({
    engines: ['google'],
    request_id: req.request_id,
  }));
  h.commit.mockReset();
  h.commit.mockResolvedValue(1);
  autoTranslateOn.set(true);
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(TranslateWindow, { target });
  await flush();
});

afterEach(() => {
  if (app) unmount(app);
  target.remove();
  vi.useRealTimers();
});

describe('TranslateWindow, mounted: translate as I type (issue #57)', () => {
  it('translates once after a pause, flagged as automatic', async () => {
    type(TEXT);
    await advance(599);
    expect(h.translate).not.toHaveBeenCalled();
    await advance(1);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0)).toMatchObject({ text: TEXT, auto: true });
  });

  it('a burst of typing is one translation, of the last text', async () => {
    type('Hola');
    await advance(300);
    type('Hola a');
    await advance(300);
    type('Hola amigo');
    await advance(599);
    expect(h.translate).not.toHaveBeenCalled();
    await advance(1);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0).text).toBe('Hola amigo');
  });

  it('a paste translates at once, through the switch flow, exactly once', async () => {
    type(TEXT, 'insertFromPaste');
    await flush();
    expect(h.plan).toHaveBeenCalledTimes(1);
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0)).toMatchObject({ text: TEXT, auto: true });
  });

  it('one or two typed characters, or blank text, translate nothing', async () => {
    type('ho');
    await advance(5000);
    type('   ');
    await advance(5000);
    await flush();
    expect(h.translate).not.toHaveBeenCalled();
  });

  it('with the switch off, typing translates nothing, and the button still does, not flagged', async () => {
    const box = target.querySelector('[data-testid="auto-translate-checkbox"]') as HTMLInputElement;
    box.click();
    await flush();
    expect(box.checked).toBe(false);
    type(TEXT);
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled();
    const btn = [...target.querySelectorAll('button')].find((b) =>
      b.className.includes('u-btn--primary'),
    ) as HTMLButtonElement;
    btn.click();
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0).auto).toBeFalsy();
  });

  it('turning the switch off cancels a translation that was waiting', async () => {
    type(TEXT);
    await advance(300);
    (target.querySelector('[data-testid="auto-translate-checkbox"]') as HTMLInputElement).click();
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled();
  });

  it('Cmd+Enter translates at once, not flagged, and replaces the wait', async () => {
    type(TEXT);
    await advance(300);
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, bubbles: true }),
    );
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0).auto).toBeFalsy();
    await advance(5000);
    expect(h.translate).toHaveBeenCalledTimes(1); // the pause did not translate it a second time
  });

  // An IME word is not final until compositionend: nothing may translate it before.
  const compose = (text: string, inputType = 'insertCompositionText') => {
    source().value = text;
    source().dispatchEvent(
      new InputEvent('input', { bubbles: true, inputType, isComposing: true }),
    );
  };
  const endComposition = () =>
    source().dispatchEvent(new Event('compositionend', { bubbles: true }));

  it('a composition never translates before it ends', async () => {
    compose('nihao');
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled();
  });

  it('a composition cancels a wait that was already pending, and the pause restarts at compositionend', async () => {
    type('Hola amigo');
    await advance(300);
    compose('Hola amigo ni'); // the user starts composing a word
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled(); // the old timer must not fire mid-composition
    compose('Hola amigo 你好'); // the last update of the composition carries the final text
    endComposition();
    await advance(599);
    expect(h.translate).not.toHaveBeenCalled();
    await advance(1);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0).text).toBe('Hola amigo 你好');
  });

  it("WebKit's commit after compositionend does not cancel the wait that compositionend started", async () => {
    compose('ni');
    source().value = 'Hola amigo 你好';
    endComposition(); // the pause starts here
    await advance(300);
    // WebKit reports the commit after compositionend, with isComposing false.
    type('Hola amigo 你好', 'insertFromComposition');
    await advance(599);
    expect(h.translate).not.toHaveBeenCalled();
    await advance(1);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(reqOf(0).text).toBe('Hola amigo 你好');
  });

  it('closing the window cancels a translation that was waiting', async () => {
    type(TEXT);
    await advance(300);
    h.fire('kai:window:closing', 'translate');
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled();
  });

  it('closing another window leaves a waiting translation alone', async () => {
    type(TEXT);
    await advance(300);
    h.fire('kai:window:closing', 'settings');
    await advance(600);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
  });

  it('unmounting the window cancels a translation that was waiting, with no request and no error', async () => {
    type(TEXT);
    await advance(300);
    unmount(app!);
    app = undefined;
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled();
  });

  it('Clear cancels a translation that was waiting', async () => {
    type(TEXT);
    await advance(300);
    const clear = [...target.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'Clear',
    );
    (clear as HTMLButtonElement).click();
    await advance(5000);
    expect(h.translate).not.toHaveBeenCalled();
  });
});

describe('TranslateWindow, mounted: the history of automatic translations (issue #57)', () => {
  it('asks the backend to write it only after a quiet moment with the text unchanged', async () => {
    type(TEXT);
    await advance(600);
    await flush();
    const id = reqOf(0).request_id;
    result(id);
    await flush();
    await advance(2499);
    expect(h.commit).not.toHaveBeenCalled();
    await advance(1);
    expect(h.commit).toHaveBeenCalledTimes(1);
    expect(h.commit).toHaveBeenCalledWith(id);
  });

  it('typing again before the quiet moment never commits the earlier request', async () => {
    type(TEXT);
    await advance(600);
    await flush();
    const first = reqOf(0).request_id;
    result(first);
    await flush();
    await advance(1000);
    type(TEXT + ' y más');
    await advance(600);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(2);
    const second = reqOf(1).request_id;
    expect(second).not.toBe(first);
    result(second);
    await flush();
    await advance(2500);
    expect(h.commit).toHaveBeenCalledTimes(1);
    expect(h.commit).toHaveBeenCalledWith(second);
  });

  it('a translation the user asked for is never committed by the window (the backend saved it)', async () => {
    type(TEXT);
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, bubbles: true }),
    );
    await flush();
    result(reqOf(0).request_id);
    await flush();
    await advance(10000);
    expect(h.commit).not.toHaveBeenCalled();
  });

  it('Clear after an automatic translation never commits it', async () => {
    type(TEXT);
    await advance(600);
    await flush();
    result(reqOf(0).request_id);
    await flush();
    await advance(1000);
    const clear = [...target.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'Clear',
    );
    (clear as HTMLButtonElement).click();
    await advance(10000);
    expect(h.commit).not.toHaveBeenCalled();
  });
});
