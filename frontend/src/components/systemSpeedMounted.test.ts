import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #10, mounted: the result notice, the once-per-request faster-setting suggestion toast,
// and the progress line's time-left — the REAL TranslateWindow.svelte in jsdom with real
// localStorage, exactly like mountedAutoTranslate.test.ts (issue #57); only the Wails boundary is
// faked. These replace the source-grep wiring checks this PR first shipped (a regexp over source
// passes even when the behavior is gone).

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
import { en } from '../i18n/en-US.ts';

const TEXT = 'Hola amigo mío';
let app: Record<string, unknown> | undefined;
let target: HTMLElement;

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
// Opens one automatic request and returns its id.
const openRequest = async () => {
  type(TEXT);
  await advance(600);
  await flush();
  return reqOf(h.translate.mock.calls.length - 1).request_id;
};
const started = (id: string, extra: Record<string, unknown> = {}) =>
  h.fire('kai:translate:progress', {
    engine: 'google',
    request_id: id,
    phase: 'started',
    started_at_ms: 0,
    ...extra,
  });
const chunk = (id: string, done: number, total: number) =>
  h.fire('kai:translate:progress', {
    engine: 'google',
    request_id: id,
    phase: 'chunk',
    done,
    total,
  });
const result = (id: string, extra: Record<string, unknown> = {}) =>
  h.fire('kai:translate:result', {
    engine: 'google',
    request_id: id,
    result: 'Hello',
    from: 'en',
    to: 'fr',
    ...extra,
  });
const toast = () => target.querySelector('.u-toast') as HTMLElement | null;
const noticeEl = () => target.querySelector('[data-testid="result-notice"]') as HTMLElement | null;
const progressParagraphs = () => [...target.querySelectorAll('p')].map((p) => p.textContent ?? '');

beforeEach(async () => {
  // setInterval is faked too (unlike mountedAutoTranslate): the progress line's 1 s tick is
  // what re-reads the (fake) clock, so advancing time moves the elapsed/remaining numbers.
  vi.useFakeTimers({
    toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'],
  });
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

describe('TranslateWindow, mounted: the result notice (issue #10)', () => {
  it('a result carrying notice shows the muted note in the result pane', async () => {
    const id = await openRequest();
    result(id, { notice: 'Faster translation needs a language pack.' });
    await flush();
    expect(noticeEl()).not.toBeNull();
    expect(noticeEl()!.textContent).toContain('Faster translation needs a language pack.');
  });

  it('a result without notice shows no note at all', async () => {
    const id = await openRequest();
    result(id);
    await flush();
    expect(noticeEl()).toBeNull();
  });

  it('a later result without a notice replaces an earlier noted one', async () => {
    const first = await openRequest();
    result(first, { notice: 'Faster translation needs a language pack.' });
    await flush();
    expect(noticeEl()).not.toBeNull();
    const second = await openRequest();
    result(second);
    await flush();
    expect(noticeEl()).toBeNull();
  });
});

describe('TranslateWindow, mounted: the faster-setting suggestion toast (issue #10)', () => {
  it('a started event with suggest_fast shows the toast, once per request id', async () => {
    const id = await openRequest();
    started(id, { estimated_ms: 180_000, suggest_fast: true });
    await flush();
    expect(toast()).not.toBeNull();
    expect(toast()!.textContent).toContain(
      en.translate.fastSuggestion.slice(0, 'This text will take'.length),
    );
    // The toast expires (12 s), and a second started event of the SAME request does not
    // bring it back: once per request.
    await advance(12_000);
    expect(toast()).toBeNull();
    started(id, { estimated_ms: 180_000, suggest_fast: true });
    await flush();
    expect(toast()).toBeNull();
  });

  it('a new request id shows the toast again', async () => {
    const id = await openRequest();
    started(id, { estimated_ms: 180_000, suggest_fast: true });
    await flush();
    await advance(12_000);
    const next = await openRequest();
    expect(next).not.toBe(id);
    started(next, { estimated_ms: 180_000, suggest_fast: true });
    await flush();
    expect(toast()).not.toBeNull();
  });

  it('clearing the input resets the once-per-request guard', async () => {
    const id = await openRequest();
    started(id, { estimated_ms: 180_000, suggest_fast: true });
    await flush();
    await advance(12_000);
    const clear = [...target.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'Clear',
    );
    (clear as HTMLButtonElement).click();
    await flush();
    const after = await openRequest();
    started(after, { estimated_ms: 180_000, suggest_fast: true });
    await flush();
    expect(toast()).not.toBeNull();
  });

  it('a started event without suggest_fast never shows the toast', async () => {
    const id = await openRequest();
    started(id, { estimated_ms: 180_000 });
    await flush();
    expect(toast()).toBeNull();
  });
});

describe('TranslateWindow, mounted: the progress line time-left (issue #10)', () => {
  it('the started estimate shows as a time-left while the engine works', async () => {
    const id = await openRequest();
    started(id, { estimated_ms: 180_000 });
    await flush();
    expect(progressParagraphs().some((p) => p.includes('about 03:00 left'))).toBe(true);
  });

  it('once parts land, the line names the part and prices the rest proportionally', async () => {
    const id = await openRequest();
    started(id, { estimated_ms: 50_000 });
    await flush();
    // 60 s in, part 1 of 4 lands: remaining = 60 s * (4-1)/1 = 03:00, overriding the estimate.
    await advance(60_000);
    chunk(id, 1, 4);
    await flush();
    expect(progressParagraphs().some((p) => p.includes('part 1 of 4'))).toBe(true);
    expect(progressParagraphs().some((p) => p.includes('about 03:00 left'))).toBe(true);
  });

  it('an engine with no estimate shows no time-left', async () => {
    const id = await openRequest();
    started(id);
    await flush();
    expect(progressParagraphs().some((p) => p.includes('left'))).toBe(false);
  });
});
