import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #208: the REAL TranslateWindow.svelte, mounted in jsdom with real localStorage, for the
// "Correct grammar and wording" checkbox and the note it leaves in the result pane. Only the Wails
// boundary is faked: the runtime, and the generated bindings. The correct binding answers what the
// backend would (its decisions are tested in Go); these tests pin what the window does with it.

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
    correct: vi.fn(),
    availability: vi.fn(),
    config: {} as Record<string, unknown>,
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
  CorrectSource: (req: unknown) => h.correct(req),
  CorrectionAvailability: () => h.availability(),
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

const ORIGINAL = 'Necesito hacer el follow up con el cliente antes del deadline';
const FIXED = 'Necesito hacer el seguimiento con el cliente antes de la fecha límite';
const CORRECTED = {
  corrected: true,
  status: 'corrected',
  reason: '',
  text: FIXED,
  original: ORIGINAL,
  language: 'es-MX',
  changes: [
    { before: 'follow up', after: 'seguimiento' },
    { before: 'del deadline', after: 'de la fecha límite' },
  ],
};

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
const checkbox = () =>
  target.querySelector('[data-testid="correct-source-checkbox"]') as HTMLInputElement;
const note = () => target.querySelector('[data-testid="source-corrected-note"]');
const originalBtn = () =>
  target.querySelector('[data-testid="translate-original"]') as HTMLButtonElement | null;
const source = () => target.querySelector('textarea') as HTMLTextAreaElement;
const translateBtn = () =>
  [...target.querySelectorAll('button')].find((b) =>
    b.className.includes('u-btn--primary'),
  ) as HTMLButtonElement;
const type = (text: string, inputType: string) => {
  source().value = text;
  source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType }));
};
/** A result lands for the request the window made last. */
const landResult = async (from = 'es-MX') => {
  const req = h.translate.mock.calls.at(-1)![0] as { request_id: string };
  h.fire('kai:translate:result', {
    engine: 'google',
    request_id: req.request_id,
    result: 'I need to do the follow-up with the client before the deadline',
    from,
    to: 'en',
  });
  await settle();
};

async function start(config: Record<string, unknown>) {
  h.config = { default_from: 'es-MX', default_to: 'en', auto_clipboard: false, ...config };
  app = mount(TranslateWindow, { target });
  await settle();
}

beforeEach(() => {
  localStorage.clear();
  h.handlers.clear();
  for (const f of [h.plan, h.learn, h.saveConfig, h.translate, h.correct, h.availability])
    f.mockReset();
  h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
  h.availability.mockResolvedValue({ available: true, reason: '' });
  h.correct.mockResolvedValue(CORRECTED);
  h.translate.mockImplementation(async (req: { request_id: string }) => ({
    engines: ['google'],
    request_id: req.request_id,
  }));
  target = document.createElement('div');
  document.body.appendChild(target);
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target.remove();
});

describe('the toolbar checkbox', () => {
  it('is there, labelled, unchecked and enabled by default', async () => {
    await start({});
    expect(checkbox()).not.toBeNull();
    expect(checkbox().checked).toBe(false);
    expect(checkbox().disabled).toBe(false);
    expect(target.textContent).toContain('Correct grammar and wording');
  });

  it('carries a tooltip in the pane-header style (u-tooltip + data-tooltip) that says what it does and that it stays on the Mac', async () => {
    await start({});
    const label = checkbox().closest('label') as HTMLElement;
    expect(label.className).toContain('u-tooltip');
    const tip = label.getAttribute('data-tooltip') ?? '';
    expect(tip).toMatch(/on-device/i);
    // One tooltip only: the native `title` would show a second, duplicate one after its delay.
    expect(label.hasAttribute('title')).toBe(false);
    // The text stays available to assistive tech: the checkbox is described by the same string.
    const ids = (checkbox().getAttribute('aria-describedby') ?? '').split(/\s+/).filter(Boolean);
    const described = ids.map((id) => target.querySelector(`#${id}`)?.textContent?.trim());
    expect(described).toContain(tip);
  });

  it('no element carrying data-tooltip also carries a native title (no double tooltip), and every one is still described', async () => {
    await start({});
    const tipped = [...target.querySelectorAll('[data-tooltip]')] as HTMLElement[];
    // the correction label, the pin button and the auto-clipboard button
    expect(tipped.length).toBeGreaterThanOrEqual(3);
    for (const el of tipped) {
      const tip = el.getAttribute('data-tooltip')!;
      expect(el.hasAttribute('title'), tip).toBe(false);
      const ctl = el.tagName === 'LABEL' ? (el.querySelector('input') as HTMLElement) : el;
      const ids = (ctl.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(Boolean);
      const accessible = [
        ctl.getAttribute('aria-label'),
        ctl.getAttribute('aria-description'),
        ...ids.map((id) => target.querySelector(`#${id}`)?.textContent?.trim()),
      ];
      expect(accessible, tip).toContain(tip);
    }
  });

  it('reads the saved setting: only an explicit true turns it on', async () => {
    await start({ correct_source_text: true });
    expect(checkbox().checked).toBe(true);
    unmount(app!);
    app = undefined;
    for (const garbled of ['yes', 1, 'true', null, {}, []]) {
      await start({ correct_source_text: garbled });
      expect(checkbox().checked, String(garbled)).toBe(false);
      unmount(app!);
      app = undefined;
      target.innerHTML = '';
    }
  });

  it('remembers a change through SaveConfig, keeping the rest of the config, and teaches nothing', async () => {
    await start({ default_engine: 'google' });
    checkbox().click();
    await settle();
    expect(checkbox().checked).toBe(true);
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({
      correct_source_text: true,
      default_from: 'es-MX',
      default_engine: 'google',
    });
    checkbox().click();
    await settle();
    expect(h.saveConfig.mock.calls[1][0]).toMatchObject({ correct_source_text: false });
    expect(h.learn).not.toHaveBeenCalled();
  });

  it('is disabled with an explanation when the provider reports unavailable, and never corrects', async () => {
    h.availability.mockResolvedValue({ available: false, reason: 'apple_intelligence_off' });
    await start({ correct_source_text: true });
    expect(checkbox().disabled).toBe(true);
    expect(checkbox().checked).toBe(false);
    const label = checkbox().closest('label') as HTMLElement;
    expect(label.getAttribute('data-tooltip')).toMatch(/Apple Intelligence/);
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    expect(h.correct).not.toHaveBeenCalled();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: ORIGINAL });
  });

  it('explains each reason in its own words', async () => {
    const seen: string[] = [];
    for (const reason of [
      'apple_intelligence_off',
      'model_not_ready',
      'unsupported_hardware',
      'unsupported_platform',
      'unavailable',
    ]) {
      h.availability.mockResolvedValue({ available: false, reason });
      await start({});
      seen.push((checkbox().closest('label') as HTMLElement).getAttribute('data-tooltip') ?? '');
      unmount(app!);
      app = undefined;
      target.innerHTML = '';
    }
    expect(new Set(seen).size).toBe(seen.length);
    for (const s of seen) expect(s.length).toBeGreaterThan(10);
  });
});

describe('a text that arrives with the checkbox on', () => {
  it('translates the corrected text, keeps the original in the source pane, and says what changed', async () => {
    await start({ correct_source_text: true });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    expect(h.correct).toHaveBeenCalledTimes(1);
    expect(h.correct.mock.calls[0][0]).toMatchObject({ text: ORIGINAL, from: 'es-MX' });
    // The switch planner reads the corrected text, and the engine is sent it.
    expect(h.plan.mock.calls[0][0]).toMatchObject({ text: FIXED });
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: FIXED, from: 'es-MX', to: 'en' });
    // The source pane still shows what the user gave.
    expect(source().value).toBe(ORIGINAL);

    await landResult();
    expect(note(), 'no correction note in the result pane').not.toBeNull();
    const text = note()!.textContent ?? '';
    expect(text).toContain('follow up');
    expect(text).toContain('seguimiento');
    expect(text).toContain('del deadline');
    expect(text).toContain('de la fecha límite');
    expect(originalBtn()).not.toBeNull();
    // One button to translate the original: exactly one control in the note.
    expect(note()!.querySelectorAll('button')).toHaveLength(1);
  });

  it('the one button translates the original instead, drops the note, and it stays the original on the next Translate', async () => {
    await start({ correct_source_text: true });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    await landResult();
    originalBtn()!.click();
    await settle();
    expect(h.translate).toHaveBeenCalledTimes(2);
    expect(h.translate.mock.calls[1][0]).toMatchObject({ text: ORIGINAL, from: 'es-MX', to: 'en' });
    expect(source().value).toBe(ORIGINAL);
    expect(note()).toBeNull();
    await landResult();
    expect(note()).toBeNull();
    // Pressing Translate again does not put the correction back.
    translateBtn().click();
    await settle();
    expect(h.translate.mock.calls.at(-1)![0]).toMatchObject({ text: ORIGINAL });
    expect(h.correct).toHaveBeenCalledTimes(1);
  });

  it('never persists the corrected text or the pair, and never teaches a variant', async () => {
    await start({ correct_source_text: true });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    await landResult();
    originalBtn()!.click();
    await settle();
    expect(h.saveConfig).not.toHaveBeenCalled();
    expect(h.learn).not.toHaveBeenCalled();
    expect(JSON.stringify(Object.entries(localStorage))).not.toContain(FIXED);
    expect(select('From').value).toBe('es-MX');
    expect(select('To').value).toBe('en');
  });

  it('an unchanged answer shows no note and translates the text as it came', async () => {
    h.correct.mockResolvedValue({
      ...CORRECTED,
      corrected: false,
      status: 'unchanged',
      text: ORIGINAL,
      changes: null,
    });
    await start({ correct_source_text: true });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    await landResult();
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: ORIGINAL });
    expect(note()).toBeNull();
  });

  it('a failing backend translates the original', async () => {
    h.correct.mockRejectedValue(new Error('boom'));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await start({ correct_source_text: true });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: ORIGINAL });
  });

  it('the checkbox off never asks the backend', async () => {
    await start({});
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    expect(h.correct).not.toHaveBeenCalled();
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: ORIGINAL });
  });

  it('an Auto source never asks the backend', async () => {
    await start({ correct_source_text: true, default_from: 'auto' });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    expect(h.correct).not.toHaveBeenCalled();
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: ORIGINAL });
  });

  it('typed text is corrected when Translate is pressed, and Cmd+Enter does the same', async () => {
    await start({ correct_source_text: true });
    type(ORIGINAL, 'insertText');
    await settle();
    expect(h.correct).not.toHaveBeenCalled();
    translateBtn().click();
    await settle();
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: FIXED });
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, bubbles: true }),
    );
    await settle();
    expect(h.translate.mock.calls.at(-1)![0]).toMatchObject({ text: FIXED });
  });

  it('a paste is corrected too, and translates only if it switched', async () => {
    await start({ correct_source_text: true, default_from: 'en', default_to: 'fr' });
    h.plan.mockResolvedValue({ switched: true, from: 'es-MX', to: 'en' });
    type(ORIGINAL, 'insertFromPaste');
    await settle();
    expect(h.correct).toHaveBeenCalledTimes(1);
    expect(h.plan.mock.calls[0][0]).toMatchObject({ text: FIXED });
    expect(select('From').value).toBe('es-MX');
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: FIXED, from: 'es-MX' });
    expect(source().value).toBe(ORIGINAL);
  });

  it('Translate is held while the model works, so a second press cannot start a second request', async () => {
    let release: (v: unknown) => void = () => {};
    h.correct.mockImplementation(
      () =>
        new Promise((r) => {
          release = r;
        }),
    );
    await start({ correct_source_text: true });
    type(ORIGINAL, 'insertText');
    await settle();
    translateBtn().click();
    await settle();
    expect(translateBtn().disabled).toBe(true);
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, bubbles: true }),
    );
    await settle();
    expect(h.correct).toHaveBeenCalledTimes(1);
    release(CORRECTED);
    await settle();
    expect(h.translate).toHaveBeenCalledTimes(1);
    expect(h.translate.mock.calls[0][0]).toMatchObject({ text: FIXED });
  });

  it('the note is gone once the text is edited', async () => {
    await start({ correct_source_text: true });
    h.fire('kai:input:fill', ORIGINAL);
    await settle();
    await landResult();
    expect(note()).not.toBeNull();
    type(ORIGINAL + ' ya', 'insertText');
    await settle();
    expect(note()).toBeNull();
  });
});
