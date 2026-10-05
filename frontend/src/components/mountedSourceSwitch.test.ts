import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #200: the REAL TranslateWindow.svelte, mounted in jsdom with real localStorage. Only the
// Wails boundary is faked: the runtime (events, window), and the generated bindings. The planner
// binding answers what the backend would for a mismatching arrival; its decisions are tested in Go.

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
import { EventCopyKeyFailed } from '../utils/events';

const SPANISH = 'Hola, necesito que me ayudes con este documento hoy';
const ENGLISH = 'Hello, I need you to help me with this document today';
let app: Record<string, unknown> | undefined;
let target: HTMLElement;

const settle = async () => {
  for (let i = 0; i < 8; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
  flushSync();
};
const select = (label: string) =>
  target.querySelector(`select[aria-label="${label}"]`) as HTMLSelectElement;
const swapBtn = () => target.querySelector('button[aria-label^="Swap"]') as HTMLButtonElement;

beforeEach(async () => {
  localStorage.clear();
  h.handlers.clear();
  h.plan.mockReset();
  h.learn.mockReset();
  h.saveConfig.mockReset();
  h.translate.mockReset();
  h.translate.mockImplementation(async (req: { request_id: string }) => {
    return { engines: ['google'], request_id: req.request_id };
  });
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(TranslateWindow, { target });
  await settle();
});

afterEach(() => {
  if (app) unmount(app);
  target.remove();
});

describe('TranslateWindow, mounted', () => {
  it('starts on the saved pair', () => {
    expect(select('From').value).toBe('en');
    expect(select('To').value).toBe('fr');
  });

  it('a mismatching fill switches the source, swaps the target, translates once, shows the cue, and a swap then swaps for real (issue #39)', async () => {
    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });

    h.fire('kai:input:fill', SPANISH);
    await settle();

    expect(h.plan).toHaveBeenCalledTimes(1);
    expect(h.plan.mock.calls[0][0]).toMatchObject({ text: SPANISH, from: 'en', to: 'fr' });
    expect(select('From').value).toBe('es-MX');
    expect(select('To').value).toBe('en');
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: SPANISH, from: 'es-MX', to: 'en' });

    // A result lands, so the result pane (which carries the cue) is shown.
    const req = h.translate.mock.calls[0][0] as { request_id: string };
    h.fire('kai:translate:result', {
      engine: 'google',
      request_id: req.request_id,
      result: ENGLISH,
      from: 'es-MX',
      to: 'en',
    });
    await settle();
    expect(target.querySelector('[data-testid="source-switched-note"]')).not.toBeNull();

    // The switch neither taught the variant store nor saved the pair.
    expect(h.learn).not.toHaveBeenCalled();
    expect(h.saveConfig).not.toHaveBeenCalled();
    expect(JSON.stringify(Object.entries(localStorage))).not.toContain('default_');

    // Issue #39: swap after an automatic switch is a real swap, not an undo of the switch. Each pane
    // ends up in the language its dropdown names: the source shows the translation (English), the
    // target is the language the text was in, and the English text is translated back to Spanish.
    swapBtn().click();
    await settle();
    expect(select('From').value).toBe('en');
    expect(select('To').value).toBe('es-MX');
    expect((target.querySelector('textarea') as HTMLTextAreaElement).value).toBe(ENGLISH);
    expect(h.translate).toHaveBeenCalledTimes(2);
    expect(h.translate.mock.calls[1][0]).toMatchObject({ text: ENGLISH, from: 'en', to: 'es-MX' });
    expect(target.querySelector('[data-testid="source-switched-note"]')).toBeNull();
    // The swap consumes preferences and never teaches the variant store.
    expect(h.learn).not.toHaveBeenCalled();
  });

  it('a fill the planner does not switch changes nothing and translates with the pinned pair', async () => {
    h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
    h.fire('kai:input:fill', SPANISH);
    await settle();
    expect(select('From').value).toBe('en');
    expect(select('To').value).toBe('fr');
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ from: 'en', to: 'fr' });
  });

  it('reports why the window did not switch to the main log (issue #16)', async () => {
    h.plan.mockResolvedValue({ switched: false, from: '', to: '', reason: 'below_confidence' });
    h.fire('kai:input:fill', SPANISH);
    await settle();
    expect(h.reportSkip).toHaveBeenCalledWith('below_confidence');
  });

  it('an Auto source never asks the planner', async () => {
    select('From').value = 'auto';
    select('From').dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    h.learn.mockReset();
    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });
    h.fire('kai:input:fill', SPANISH);
    await settle();
    expect(h.plan).not.toHaveBeenCalled();
    expect(select('From').value).toBe('auto');
    expect(h.translate).toHaveBeenCalledTimes(1);
  });

  const source = () => target.querySelector('textarea') as HTMLTextAreaElement;
  const type = (text: string, inputType: string) => {
    source().value = text;
    source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType }));
  };
  const translateBtn = () =>
    [...target.querySelectorAll('button')].find((b) =>
      b.className.includes('u-btn--primary'),
    ) as HTMLButtonElement;

  it('typed text is checked when Translate is pressed, not per keystroke', async () => {
    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });
    type(SPANISH, 'insertText');
    await settle();
    expect(h.plan).not.toHaveBeenCalled();
    expect(select('From').value).toBe('en');
    translateBtn().click();
    await settle();
    expect(h.plan).toHaveBeenCalledTimes(1);
    expect(select('From').value).toBe('es-MX');
    expect(select('To').value).toBe('en');
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ from: 'es-MX', to: 'en' });
  });

  it('Cmd+Enter switches like the button', async () => {
    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });
    type(SPANISH, 'insertText');
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, bubbles: true }),
    );
    await settle();
    expect(select('From').value).toBe('es-MX');
    expect(h.translate).toHaveBeenCalledTimes(1);
  });

  it('a paste translates only if it switched', async () => {
    h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
    type(SPANISH, 'insertFromPaste');
    await settle();
    expect(h.plan).toHaveBeenCalledTimes(1);
    expect(h.translate).not.toHaveBeenCalled();

    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });
    type(SPANISH + ' otra vez', 'insertFromPaste');
    await settle();
    expect(select('From').value).toBe('es-MX');
    expect(h.translate).toHaveBeenCalledTimes(1);
  });
});

describe('TranslateWindow, mounted: double Cmd+C (issue #199)', () => {
  it('a double-copy text arrives through the same EventInputFill path and switches the source', async () => {
    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });
    // The backend emits exactly this event for a double Cmd+C (as for the auto-clipboard fill).
    h.fire('kai:input:fill', SPANISH);
    await settle();
    expect(h.plan).toHaveBeenCalledTimes(1);
    expect(select('From').value).toBe('es-MX');
    expect(h.translate).toHaveBeenCalledTimes(1);
  });

  it('a missing Input Monitoring grant shows the actionable toast', async () => {
    expect(target.querySelector('.u-toast')).toBeNull();
    h.fire('kai:doublecopy:permission-missing', undefined);
    await settle();
    const toast = target.querySelector('.u-toast');
    expect(toast, 'no toast after kai:doublecopy:permission-missing').not.toBeNull();
    expect(toast!.textContent).toContain('Input Monitoring');
    expect(toast!.textContent).toContain('Privacy & Security');
  });
});

describe('TranslateWindow, mounted: missing Accessibility permission (issue #194)', () => {
  it('shows the actionable toast, naming the pane, not the generic failure', async () => {
    expect(target.querySelector('.u-toast')).toBeNull();
    h.fire('kai:accessibility:missing', undefined);
    await settle();
    const toast = target.querySelector('.u-toast');
    expect(toast, 'no toast after kai:accessibility:missing').not.toBeNull();
    const text = toast!.textContent ?? '';
    expect(text).toContain('Device Control and Data Access');
    expect(text).toContain('Privacy & Security');
    expect(text).toContain('Kai');
    expect(text).not.toContain("couldn't capture");
  });

  it('the ordinary copy-key failure keeps its own, different message', async () => {
    h.fire(EventCopyKeyFailed, undefined);
    await settle();
    const text = target.querySelector('.u-toast')?.textContent ?? '';
    expect(text).not.toContain('Accessibility');
  });
});
