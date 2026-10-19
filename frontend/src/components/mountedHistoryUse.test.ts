import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #60: the translate window's side of the History screen, with the REAL TranslateWindow.svelte
// mounted in jsdom: the History button and its shortcut open the History window, and "Use" (the
// EventHistoryUse the History window sends) puts an entry back in the panes without translating.
// Only the Wails boundary is faked, as in mountedAutoTranslate.test.ts.

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(e: { data: unknown }) => void>>();
  return {
    handlers,
    fire(name: string, data: unknown) {
      for (const cb of handlers.get(name) ?? []) cb({ data });
    },
    translate: vi.fn(),
    showHistory: vi.fn(async () => {}),
    showSettings: vi.fn(),
    cancel: vi.fn(async (..._a: unknown[]) => true),
    saveConfig: vi.fn(),
    backTranslate: vi.fn(async (..._a: unknown[]) => null),
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
  CancelTranslate: (...a: unknown[]) => h.cancel(...a),
  PlanSourceSwitch: async () => ({ switched: false, from: '', to: '' }),
  ReportSourceSwitchSkipped: async () => {},
  CorrectSource: vi.fn(),
  CommitAutoHistory: vi.fn(async () => 1),
  BackTranslate: (...a: unknown[]) => h.backTranslate(...a),
  CorrectionAvailability: async () => ({ available: true, reason: '' }),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts', () => ({
  Learn: vi.fn(),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts', () => ({
  ShowSettings: () => h.showSettings(),
  ShowHistory: () => h.showHistory(),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts', () => {
  const engine = (value: string) => ({ value, name: value, kind: 'translate', supported: true });
  return {
    GetEngines: async () => [engine('system'), engine('google')],
    GetAllEngines: async () =>
      ['system', 'google'].map((value, id) => ({
        id,
        value,
        name: value,
        kind: 'translate',
        enabled: true,
        supported: true,
        target_languages: ['en', 'fr', 'es-MX'],
      })),
  };
});
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetLanguages: async () => ['auto', 'en', 'fr', 'es-MX'].map((v) => ({ value: v, name: v })),
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: (c: unknown) => h.saveConfig(c),
  GetTheme: async () => 'light',
  SetTheme: async () => {},
}));

import TranslateWindow from './TranslateWindow.svelte';
import { autoTranslateOn } from '../stores/autoTranslate';
import { backTranslateOn } from '../stores/backTranslate';
import { historyShortcut } from '../stores/historyShortcut';

const ENTRY = {
  text: 'Necesito los puntos clave de tus cambios',
  result: 'I need the key points of your changes',
  from_lang: 'es-MX',
  to_lang: 'en',
  engine: 'google',
};
let app: Record<string, unknown> | undefined;
let target: HTMLElement;

const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const source = () => target.querySelectorAll('textarea')[0] as HTMLTextAreaElement;
const resultPane = () => target.querySelectorAll('textarea')[1] as HTMLTextAreaElement;
const select = (label: string) =>
  target.querySelector(`select[aria-label="${label}"]`) as HTMLSelectElement;
const keydown = (init: KeyboardEventInit) =>
  window.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }));

beforeEach(async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  localStorage.clear();
  h.handlers.clear();
  h.translate.mockReset();
  h.translate.mockImplementation(async (req: { request_id: string }) => ({
    engines: ['system'],
    request_id: req.request_id,
  }));
  h.showHistory.mockClear();
  h.cancel.mockClear();
  h.saveConfig.mockClear();
  h.backTranslate.mockClear();
  autoTranslateOn.set(false);
  backTranslateOn.set(false);
  historyShortcut.set('Cmd+Y');
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

describe('TranslateWindow, mounted: opening History (issue #60)', () => {
  it('has a History button that opens the History window and nothing else', async () => {
    const btn = target.querySelector('[data-testid="open-history"]') as HTMLButtonElement;
    expect(btn).not.toBeNull();
    btn.click();
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
    expect(h.showSettings).not.toHaveBeenCalled();
  });

  it('names the shortcut in the button tooltip', () => {
    const btn = target.querySelector('[data-testid="open-history"]') as HTMLButtonElement;
    expect(btn.title).toContain('Cmd+Y');
  });

  it('Cmd+Y opens it, and the key is taken from the page', async () => {
    const ev = new KeyboardEvent('keydown', { key: 'y', metaKey: true, cancelable: true });
    window.dispatchEvent(ev);
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
    expect(ev.defaultPrevented).toBe(true);
  });

  it('other keys do not open it', async () => {
    keydown({ key: 'y' });
    keydown({ key: 'y', metaKey: true, shiftKey: true });
    keydown({ key: 'h', metaKey: true });
    await flush();
    expect(h.showHistory).not.toHaveBeenCalled();
  });

  it('follows a shortcut saved in Settings, and no shortcut means no key', async () => {
    h.fire('kai:history:shortcut-changed', 'Cmd+Shift+H');
    await flush();
    keydown({ key: 'y', metaKey: true });
    expect(h.showHistory).not.toHaveBeenCalled();
    keydown({ key: 'h', metaKey: true, shiftKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);

    h.fire('kai:history:shortcut-changed', '');
    await flush();
    keydown({ key: 'h', metaKey: true, shiftKey: true });
    await flush();
    expect(h.showHistory).toHaveBeenCalledTimes(1);
  });
});

describe('TranslateWindow, mounted: Use from History (issue #60)', () => {
  it('puts the source, the result, both languages and the engine in the panes', async () => {
    h.fire('kai:history:use', ENTRY);
    await flush();
    expect(source().value).toBe(ENTRY.text);
    expect(resultPane().value).toBe(ENTRY.result);
    expect(select('From').value).toBe('es-MX');
    expect(select('To').value).toBe('en');
    expect(JSON.parse(localStorage.getItem('kai:translate:lastEngine') ?? '""')).toBe('google');
  });

  it('does not translate, and asks the backend for nothing', async () => {
    h.fire('kai:history:use', ENTRY);
    await flush();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(h.translate).not.toHaveBeenCalled();
    expect(h.backTranslate).not.toHaveBeenCalled();
  });

  it('leaves "Translate as I type" and back-translation as they were', async () => {
    autoTranslateOn.set(true);
    backTranslateOn.set(false);
    h.fire('kai:history:use', ENTRY);
    await flush();
    let auto = false;
    let back = true;
    autoTranslateOn.subscribe((v) => (auto = v))();
    backTranslateOn.subscribe((v) => (back = v))();
    expect(auto).toBe(true);
    expect(back).toBe(false);
  });

  it('with back-translation on, the restored result is checked back as any shown result is', async () => {
    backTranslateOn.set(true);
    h.fire('kai:history:use', ENTRY);
    await flush();
    // The same rule as for a fresh translation (issue #56), not a translation of the entry itself.
    expect(h.translate).not.toHaveBeenCalled();
  });

  it('drops a translation that was waiting for its pause', async () => {
    autoTranslateOn.set(true);
    source().value = 'Hola amigo mío';
    source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await vi.advanceTimersByTimeAsync(300);
    h.fire('kai:history:use', ENTRY);
    await flush();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(h.translate).not.toHaveBeenCalled();
    expect(source().value).toBe(ENTRY.text);
  });

  it('cancels a request that is still open and ignores its late result', async () => {
    source().value = 'Hola amigo mío';
    source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await flush();
    const btn = [...target.querySelectorAll('button')].find((b) =>
      b.className.includes('u-btn--primary'),
    ) as HTMLButtonElement;
    btn.click();
    await flush();
    const open = (h.translate.mock.calls[0][0] as { request_id: string }).request_id;

    h.fire('kai:history:use', ENTRY);
    await flush();
    expect(h.cancel).toHaveBeenCalledWith(open, '');

    h.fire('kai:translate:result', {
      engine: 'system',
      request_id: open,
      result: 'late',
      from: 'es',
      to: 'en',
    });
    await flush();
    expect(resultPane().value).toBe(ENTRY.result);
  });

  it('can be undone like any other change to the source text', async () => {
    source().value = 'before';
    source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
    await flush();
    h.fire('kai:history:use', ENTRY);
    await flush();
    keydown({ key: 'z', metaKey: true });
    await flush();
    expect(source().value).toBe('before');
  });

  it('shows the result under the engine now selected when the entry engine is gone', async () => {
    h.fire('kai:history:use', { ...ENTRY, engine: 'deepl' });
    await flush();
    expect(resultPane().value).toBe(ENTRY.result);
  });

  it('keeps a language the select cannot hold as it was', async () => {
    h.fire('kai:history:use', { ...ENTRY, from_lang: 'xx', to_lang: '' });
    await flush();
    expect(select('From').value).toBe('en');
    expect(select('To').value).toBe('fr');
    expect(source().value).toBe(ENTRY.text);
  });

  it('saves the languages like a swap does, and says it was restored', async () => {
    h.fire('kai:history:use', ENTRY);
    await flush();
    expect(h.saveConfig).toHaveBeenCalledWith(
      expect.objectContaining({ default_from: 'es-MX', default_to: 'en' }),
    );
    expect(target.textContent).toContain('Restored from history');
  });

  it('ignores a payload that is not an entry', async () => {
    h.fire('kai:history:use', null);
    h.fire('kai:history:use', {});
    await flush();
    expect(source().value).toBe('');
  });
});
