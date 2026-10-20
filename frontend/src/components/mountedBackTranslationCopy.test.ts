import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { en } from '../i18n/en-US.ts';

// Issue #8 (collector): the back-translation at the bottom of the translation pane gets the
// same copy control the result above has. The REAL TranslateWindow.svelte, mounted in jsdom
// with real localStorage, like mountedSourceCorrection.test.ts: only the Wails boundary is
// faked (the runtime and the generated bindings). BackTranslate answers what the backend
// would (its decisions are tested in Go); these tests pin what the window does with it.

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
    back: vi.fn(),
    setClipboard: vi.fn(async (_text: string) => {}),
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
  Clipboard: { SetText: (text: string) => h.setClipboard(text) },
  System: { IsDarkMode: async () => false },
}));

vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts', () => ({
  TranslateMulti: (req: unknown) => h.translate(req),
  CancelTranslate: vi.fn(async () => true),
  PlanSourceSwitch: (req: unknown) => h.plan(req),
  ReportSourceSwitchSkipped: (reason: string) => h.reportSkip(reason),
  CorrectSource: vi.fn(),
  CommitAutoHistory: vi.fn(async () => {}),
  CorrectionAvailability: async () => ({ available: true, reason: '' }),
  RetranslateWithContext: vi.fn(async () => {}),
  BackTranslate: (req: unknown) => h.back(req),
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
import { backTranslateOn } from '../stores/backTranslate';

const SOURCE = 'Necesito hacer el seguimiento con el cliente';
const RESULT = 'I need to follow up with the client';
const BACK = 'Tengo que hacer el seguimiento con el cliente';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;

const settle = async () => {
  for (let i = 0; i < 8; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
  flushSync();
};
const source = () => target.querySelector('textarea') as HTMLTextAreaElement;
const translateBtn = () =>
  [...target.querySelectorAll('button')].find((b) =>
    b.className.includes('u-btn--primary'),
  ) as HTMLButtonElement;
const backBlock = () => target.querySelector('[data-testid="back-translation"]');
const backCopyBtn = () =>
  target.querySelector('[data-testid="back-translation-copy"]') as HTMLButtonElement | null;
const headerCopyBtn = () =>
  target.querySelector(`button[aria-label="${en.translate.copy}"]`) as HTMLButtonElement | null;
const toast = () => target.querySelector('.u-toast');

/** A result lands for the request the window made last. */
const landResult = async () => {
  const req = h.translate.mock.calls.at(-1)![0] as { request_id: string };
  h.fire('kai:translate:result', {
    engine: 'google',
    request_id: req.request_id,
    result: RESULT,
    from: 'es-MX',
    to: 'en',
  });
  await settle();
};

/** Drive one full translate: typed text, Translate pressed, result and back-translation landed. */
const translateWithBack = async () => {
  source().value = SOURCE;
  source().dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
  // Let the typed text reach the component's state before the button reads it.
  await settle();
  translateBtn().click();
  await settle();
  await landResult();
};

async function start(config: Record<string, unknown>) {
  h.config = { default_from: 'es-MX', default_to: 'en', auto_clipboard: false, ...config };
  app = mount(TranslateWindow, { target });
  await settle();
}

beforeEach(() => {
  localStorage.clear();
  // The switch under test needs the back-translation feature on; its own default and
  // persistence are pinned by backTranslationWiring.test.ts.
  backTranslateOn.set(true);
  h.handlers.clear();
  for (const f of [h.plan, h.learn, h.saveConfig, h.translate, h.back, h.setClipboard])
    f.mockReset();
  h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
  h.translate.mockImplementation(async (req: { request_id: string }) => ({
    engines: ['google'],
    request_id: req.request_id,
  }));
  h.back.mockImplementation(async (req: { request_id: string }) => ({
    status: 'ok',
    result: BACK,
    engine: 'google',
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

describe('the back-translation copy control (issue #8)', () => {
  it('sits in the back-translation block as an icon button with its own tooltip', async () => {
    await start({});
    await translateWithBack();
    expect(backBlock()).not.toBeNull();
    const btn = backCopyBtn();
    expect(btn, 'the back-translation block must contain a copy button').not.toBeNull();
    expect(backBlock()!.contains(btn!)).toBe(true);
    // Same control family as the result's copy: an icon button, tooltip on both the
    // accessible name and the native title, an SVG inside.
    expect(btn!.className).toContain('u-icon-btn');
    expect(btn!.getAttribute('aria-label')).toBe(en.translate.copyBack);
    expect(btn!.getAttribute('title')).toBe(en.translate.copyBack);
    expect(btn!.querySelector('svg')).not.toBeNull();
  });

  it('copies exactly the back-translation — never the translation, never the source', async () => {
    await start({});
    await translateWithBack();
    backCopyBtn()!.click();
    await settle();
    expect(h.setClipboard).toHaveBeenCalledTimes(1);
    expect(h.setClipboard).toHaveBeenCalledWith(BACK);
    const copied = h.setClipboard.mock.calls.map((c) => c[0]);
    expect(copied).not.toContain(RESULT);
    expect(copied).not.toContain(SOURCE);
    // The pane-header Copy keeps copying the translation above, unchanged.
    headerCopyBtn()!.click();
    await settle();
    expect(h.setClipboard).toHaveBeenCalledTimes(2);
    expect(h.setClipboard).toHaveBeenLastCalledWith(RESULT);
  });

  it('shows the same Copied toast and resets after the usual delay', async () => {
    await start({});
    await translateWithBack();
    backCopyBtn()!.click();
    await settle();
    expect(toast()?.textContent).toBe(en.common.copied);
    // showToast's default duration is 1600ms; wait it out with real timers.
    await new Promise((r) => setTimeout(r, 1800));
    await settle();
    expect(toast()).toBeNull();
  });

  it('is absent while the back-translation is pending, and appears when it lands', async () => {
    h.back.mockImplementation(() => new Promise(() => {}));
    await start({});
    await translateWithBack();
    expect(target.querySelector('[data-testid="back-translation-pending"]')).not.toBeNull();
    expect(backCopyBtn()).toBeNull();
  });

  it('is absent when the answer is not a usable back-translation', async () => {
    h.back.mockResolvedValue({ status: 'failed', result: '', engine: 'google', request_id: 'x' });
    await start({});
    await translateWithBack();
    expect(backBlock()).toBeNull();
    expect(backCopyBtn()).toBeNull();
  });

  it('is absent with the toggle off, and nothing is even asked of the backend', async () => {
    backTranslateOn.set(false);
    await start({});
    await translateWithBack();
    expect(h.back).not.toHaveBeenCalled();
    expect(backBlock()).toBeNull();
    expect(backCopyBtn()).toBeNull();
  });
});
