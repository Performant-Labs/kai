import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #60: "Use" on a history entry, with the REAL TranslateWindow.svelte mounted in jsdom.
// Only the Wails boundary is faked (the runtime, the generated bindings). This pins the
// restore itself: the source, the result, the languages and the engine land in the panes, and
// NOTHING is translated until the user asks — not the main translation, not a back-translation,
// whatever the toggles say. Also: the History button and Cmd+Y summon the history window, and
// the history window closing puts this window's pin back.

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(e: { data: unknown }) => void>>();
  return {
    handlers,
    fire(name: string, data: unknown) {
      for (const cb of handlers.get(name) ?? []) cb({ data });
    },
    emit: vi.fn(),
    plan: vi.fn(),
    reportSkip: vi.fn(async (_reason: string) => {}),
    translate: vi.fn(),
    back: vi.fn(),
    commit: vi.fn(),
    learn: vi.fn(),
    saveConfig: vi.fn(async (_c: unknown) => {}),
    showSettings: vi.fn(),
    showHistory: vi.fn(),
    setAlwaysOnTop: vi.fn(async (_b: boolean) => {}),
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
    Emit: (...args: unknown[]) => h.emit(...(args as [string, unknown])),
  },
  Window: {
    SetAlwaysOnTop: (b: boolean) => h.setAlwaysOnTop(b),
    Name: async () => 'translate',
  },
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
  BackTranslate: (req: unknown) => h.back(req),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts', () => ({
  Learn: (l: string) => h.learn(l),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts', () => ({
  ShowSettings: () => h.showSettings(),
  ShowHistory: () => h.showHistory(),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts', () => ({
  GetEngines: async () => [
    { value: 'google', name: 'Google', kind: 'translate', supported: true },
    { value: 'openai', name: 'OpenAI', kind: 'translate', supported: true },
  ],
  GetAllEngines: async () => [
    {
      id: 1,
      value: 'google',
      name: 'Google',
      kind: 'translate',
      enabled: true,
      supported: true,
      target_languages: ['en', 'fr', 'es'],
    },
    {
      id: 2,
      value: 'openai',
      name: 'OpenAI',
      kind: 'translate',
      enabled: true,
      supported: true,
      target_languages: ['en', 'fr', 'es'],
    },
  ],
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetLanguages: async () => ['auto', 'en', 'fr', 'es'].map((v) => ({ value: v, name: v })),
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: (c: unknown) => h.saveConfig(c),
  GetTheme: async () => 'light',
  SetTheme: async () => {},
}));

import TranslateWindow from './TranslateWindow.svelte';
import { backTranslateOn } from '../stores/backTranslate';
import { historyShortcut } from '../stores/historyShortcut';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
let visibility: 'visible' | 'hidden' = 'visible';

const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const sourceEl = () => target.querySelector('textarea') as HTMLTextAreaElement;
const resultEl = () => target.querySelectorAll('textarea')[1] as HTMLTextAreaElement;
const fromSel = () => target.querySelector('select[aria-label="From"]') as HTMLSelectElement;
const toSel = () => target.querySelector('select[aria-label="To"]') as HTMLSelectElement;
const key = (k: string, opts: KeyboardEventInit = {}) => {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, ...opts }));
};

const ENTRY = {
  text: 'Necesito los puntos clave de tus cambios',
  result: 'I need the key points of your changes',
  from_lang: 'es',
  to_lang: 'en',
  engine: 'openai',
};

async function mountWindow() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(TranslateWindow, { target });
  await flush();
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  localStorage.clear();
  // The persisted history-shortcut store outlives localStorage.clear (its in-memory value is
  // set at import time); every test starts from the default Cmd+Y.
  historyShortcut.set('Cmd+Y');
  h.handlers.clear();
  h.emit.mockReset();
  h.plan.mockReset();
  h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
  h.translate.mockReset();
  h.translate.mockImplementation(async (req: { request_id: string }) => ({
    engines: ['google'],
    request_id: req.request_id,
  }));
  h.back.mockReset();
  h.back.mockImplementation(async () => null);
  h.learn.mockReset();
  h.showHistory.mockReset();
  h.showSettings.mockReset();
  h.setAlwaysOnTop.mockReset();
  visibility = 'visible';
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => visibility,
  });
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.useRealTimers();
});

describe('TranslateWindow, mounted: using a history entry (issue #60)', () => {
  it('restores the source, the result, the languages and the engine, and translates nothing', async () => {
    await mountWindow();
    h.fire('kai:history:use', ENTRY);
    await flush();

    // The source pane holds the entry's text (as its own undo step: Undo brings the old text
    // back).
    expect(sourceEl().value).toBe(ENTRY.text);
    // The result pane shows the stored translation as the engine's finished result.
    expect(resultEl().value).toBe(ENTRY.result);
    // The two selects hold the entry's pair, and the engine selection follows it.
    expect(fromSel().value).toBe('es');
    expect(toSel().value).toBe('en');
    expect(localStorage.getItem('kai:translate:lastEngine')).toBe(JSON.stringify('openai'));
    // Nothing was translated: no request, and the pair was only persisted, never taught.
    expect(h.translate).not.toHaveBeenCalled();
    expect(h.learn).not.toHaveBeenCalled();
    expect(h.saveConfig).toHaveBeenCalled(); // persistLangs wrote the restored pair
    // Undo brings back the text the restore replaced.
    key('z', { metaKey: true });
    await flush();
    expect(sourceEl().value).toBe('');
  });

  it('holds the back-translation off while the restored result is on screen, toggle or not', async () => {
    backTranslateOn.set(true);
    await mountWindow();
    // Sanity: the toggle is on.
    expect(backTranslateOn).toBeTruthy();
    h.fire('kai:history:use', ENTRY);
    await flush();
    expect(resultEl().value).toBe(ENTRY.result);
    // "Use" translates nothing — a back-translation included.
    expect(h.back).not.toHaveBeenCalled();
    expect(h.translate).not.toHaveBeenCalled();
    // The hold lifts when the user asks for a translation: type + auto pause fires one.
    sourceEl().value = 'otra cosa';
    sourceEl().dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await vi.advanceTimersByTimeAsync(3000);
    await flush();
    expect(h.translate).toHaveBeenCalled();
  });

  it('an entry whose pair the selects cannot hold keeps the current pair', async () => {
    await mountWindow();
    expect(fromSel().value).toBe('en'); // the default from settings
    h.fire('kai:history:use', { ...ENTRY, from_lang: 'zz', to_lang: 'es' });
    await flush();
    // 'zz' is not an option: the source select keeps what it showed; 'es' is, so it moves.
    expect(fromSel().value).toBe('en');
    expect(toSel().value).toBe('es');
    expect(resultEl().value).toBe(ENTRY.result);
  });

  it('a garbage payload restores nothing', async () => {
    await mountWindow();
    h.fire('kai:history:use', { text: 42 });
    h.fire('kai:history:use', null);
    await flush();
    expect(sourceEl().value).toBe('');
    // The pane stays idle: there is no result textarea at all.
    expect(target.querySelectorAll('textarea')).toHaveLength(1);
  });

  it('brings itself forward only when it was hidden (Use from the Settings tab)', async () => {
    await mountWindow();
    h.emit.mockClear();
    h.fire('kai:history:use', ENTRY);
    await flush();
    // Visible (the history window case): no window-show emit, no focus stealing.
    expect(h.emit).not.toHaveBeenCalledWith('kai:window:show', 'translate');
    visibility = 'hidden';
    h.fire('kai:history:use', { ...ENTRY, text: 'otro' });
    await flush();
    expect(h.emit).toHaveBeenCalledWith('kai:window:show', 'translate');
  });

  it('the History button and Cmd+Y open the history window; Ctrl+Y stays redo', async () => {
    await mountWindow();
    (target.querySelector('[data-testid="history-button"]') as HTMLButtonElement).click();
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
    key('y', { metaKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(2);
    // Ctrl+Y (the redo convention) does not open history.
    key('y', { ctrlKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(2);
  });

  it("the History button's accessible name is the localized word, not the raw key (PR #67 review)", async () => {
    // translate.history must exist in the active catalog: t() falls back to the dotted path
    // itself for a missing key, so this pins the key's presence (and fails as
    // 'translate.history' the moment it is dropped from a locale).
    await mountWindow();
    const btn = target.querySelector('[data-testid="history-button"]') as HTMLButtonElement;
    expect(btn.getAttribute('aria-label')).toBe('History');
    expect(btn.getAttribute('title')).toBe('Browse past translations (Cmd+Y)');
  });

  it('a shortcut saved in Settings opens it, and the default stops firing (issue #60)', async () => {
    historyShortcut.set('Alt+H');
    await mountWindow();
    key('h', { altKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
    // The default no longer belongs to the window.
    key('y', { metaKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
  });

  it('an empty shortcut means no key opens it; the button still does', async () => {
    historyShortcut.set('');
    await mountWindow();
    key('y', { metaKey: true });
    key('h', { altKey: true });
    await flush();
    expect(h.showHistory).not.toHaveBeenCalled();
    (target.querySelector('[data-testid="history-button"]') as HTMLButtonElement).click();
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
    // The tooltip says what it does without naming a key.
    const btn = target.querySelector('[data-testid="history-button"]') as HTMLButtonElement;
    expect(btn.getAttribute('title')).toBe('Browse past translations');
  });

  it('follows a shortcut saved while the window is open', async () => {
    await mountWindow();
    h.fire('kai:history:shortcut-changed', 'Alt+H');
    await flush();
    key('h', { altKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
    key('y', { metaKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
  });

  it('the history window closing puts this window’s pin back', async () => {
    // Issue #69: the pin default is no longer "on" — seed an explicit stored choice so this
    // test still exercises what it is about (the closing event restores THE persisted pin).
    localStorage.setItem('kai:translate:pinned', JSON.stringify(true));
    await mountWindow();
    h.setAlwaysOnTop.mockClear();
    h.fire('kai:window:closing', 'history');
    await flush();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);
    // Another window's closing does not touch the pin.
    h.setAlwaysOnTop.mockClear();
    h.fire('kai:window:closing', 'screenshot');
    await flush();
    expect(h.setAlwaysOnTop).not.toHaveBeenCalled();
  });

  it('a translation still running is stopped when an entry is used', async () => {
    await mountWindow();
    // Start a translation the usual way.
    sourceEl().value = 'algo en marcha';
    sourceEl().dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await vi.advanceTimersByTimeAsync(3000);
    await flush();
    expect(h.translate).toHaveBeenCalledTimes(1);
    // Use an entry while the request is open: the request is cancelled, the restore lands.
    h.fire('kai:history:use', ENTRY);
    await flush();
    expect(resultEl().value).toBe(ENTRY.result);
    // The stale request's late result (its request_id no longer matches) cannot land.
    h.fire('kai:translate:result', {
      engine: 'openai',
      request_id: 'stale',
      result: 'LATE RESULT',
      from: 'es',
      to: 'en',
    });
    await flush();
    expect(resultEl().value).toBe(ENTRY.result);
  });
});
