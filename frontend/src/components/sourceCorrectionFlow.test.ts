import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createSourceSwitcher, type SourceSwitchDeps } from './sourceSwitchFlow.ts';
import type { AppliedCorrection, CorrectionAnswer } from '../utils/sourceCorrection.ts';
import type { SourceSwitchPlan } from '../utils/sourceSwitch.ts';

// Issue #208: the correction step of the arrival flow is EXECUTED here, with fakes for only the
// Wails boundary (the correct and plan bindings) and the translate callback. What the backend
// decides (setting, language, guards, diff) is tested in Go; here the backend answers what it is
// told to, and the tests pin what the window does with it: correct first, then the #200 switch and
// the translation on the corrected text, the original kept and restorable, nothing persisted.

const ORIGINAL = 'Necesito hacer el follow up con el cliente antes del deadline';
const FIXED = 'Necesito hacer el seguimiento con el cliente antes de la fecha límite';
const CORRECTED: CorrectionAnswer = {
  corrected: true,
  status: 'corrected',
  text: FIXED,
  original: ORIGINAL,
  language: 'es-MX',
  changes: [
    { before: 'follow up', after: 'seguimiento' },
    { before: 'del deadline', after: 'de la fecha límite' },
  ],
};
const NOTHING: CorrectionAnswer = {
  corrected: false,
  status: 'unchanged',
  text: ORIGINAL,
  original: ORIGINAL,
  changes: null,
};
const NO_SWITCH: SourceSwitchPlan = { switched: false, from: '', to: '' };

function setup(
  opts: {
    answer?: CorrectionAnswer | Error;
    enabled?: boolean;
    from?: string;
    plan?: SourceSwitchPlan;
    text?: string;
  } = {},
) {
  const state = {
    text: opts.text ?? ORIGINAL,
    from: opts.from ?? 'es-MX',
    to: 'en',
    enabled: opts.enabled ?? true,
  };
  const applied: (AppliedCorrection | null)[] = [];
  const busy: boolean[] = [];
  const errors: unknown[] = [];
  const order: string[] = [];
  const correct = vi.fn(async () => {
    order.push('correct');
    const a = opts.answer ?? CORRECTED;
    if (a instanceof Error) throw a;
    return a;
  });
  const plan = vi.fn(async () => {
    order.push('plan');
    return opts.plan ?? NO_SWITCH;
  });
  const translate = vi.fn(() => {
    order.push('translate');
  });
  const deps: SourceSwitchDeps = {
    plan,
    getText: () => state.text,
    getPair: () => ({ from: state.from, to: state.to }),
    setPair: (p) => {
      state.from = p.from;
      state.to = p.to;
    },
    autoCode: 'auto',
    sourceOptions: () => ['auto', 'en', 'fr', 'es-MX'],
    canBeTarget: (c) => ['en', 'fr', 'es-MX'].includes(c),
    translate,
    onCue: () => {},
    onError: (e) => errors.push(e),
    correct,
    isCorrectionEnabled: () => state.enabled,
    onCorrection: (c) => applied.push(c),
    onCorrecting: (b) => busy.push(b),
  };
  return {
    state,
    applied,
    busy,
    errors,
    order,
    correct,
    plan,
    translate,
    sw: createSourceSwitcher(deps),
  };
}

let writes: string[];
beforeEach(() => {
  localStorage.clear();
  writes = [];
  for (const m of ['setItem', 'removeItem', 'clear'] as const) {
    vi.spyOn(Storage.prototype, m).mockImplementation(function (this: Storage, ...a: unknown[]) {
      writes.push(`${m}:${String(a[0])}`);
    } as never);
  }
});

describe('a text that arrives with the correction on', () => {
  it('corrects first, then plans the switch on the corrected text, then translates', async () => {
    const { order, correct, plan, sw } = setup();
    sw.newArrival();
    await sw.translateWithSwitch();
    expect(order).toEqual(['correct', 'plan', 'translate']);
    expect(correct).toHaveBeenCalledWith({ text: ORIGINAL, from: 'es-MX', detected: '' });
    // #200's planner reads the corrected text, not the original.
    expect(plan).toHaveBeenCalledWith(
      expect.objectContaining({ text: FIXED, from: 'es-MX', to: 'en' }),
    );
  });

  it('hands the window the correction, with the original kept and what changed', async () => {
    const { applied, sw } = setup();
    await sw.translateWithSwitch();
    expect(applied.at(-1)).toEqual({
      original: ORIGINAL,
      text: FIXED,
      language: 'es-MX',
      changes: CORRECTED.changes,
    });
  });

  it('a switch planned on the corrected text still switches the pair', async () => {
    const { state, sw } = setup({ from: 'en', plan: { switched: true, from: 'es-MX', to: 'en' } });
    await sw.translateWithSwitch();
    expect(state).toMatchObject({ from: 'es-MX', to: 'en' });
  });

  it('is busy while the model works, and not after', async () => {
    const { busy, sw } = setup();
    await sw.translateWithSwitch();
    expect(busy).toEqual([true, false]);
  });

  it('does not correct again for the same text (a paste, then Translate; the engine-detection retry)', async () => {
    const { correct, translate, sw } = setup({
      plan: { switched: true, from: 'es-MX', to: 'en' },
      from: 'en',
    });
    await sw.translateIfSwitched(ORIGINAL);
    await sw.translateWithSwitch();
    expect(correct).toHaveBeenCalledTimes(1);
    expect(translate).toHaveBeenCalled();
  });

  it('asks again for another text, and for the same text under another source', async () => {
    const { state, correct, sw } = setup();
    await sw.translateWithSwitch();
    state.text = ORIGINAL + ' hoy';
    await sw.translateWithSwitch();
    state.text = ORIGINAL;
    state.from = 'fr';
    await sw.translateWithSwitch();
    expect(correct).toHaveBeenCalledTimes(3);
  });
});

describe('when there is nothing to correct with', () => {
  it('the checkbox off never asks the backend and plans on the original text', async () => {
    const { correct, plan, applied, sw } = setup({ enabled: false });
    await sw.translateWithSwitch();
    expect(correct).not.toHaveBeenCalled();
    expect(plan).toHaveBeenCalledWith(expect.objectContaining({ text: ORIGINAL }));
    expect(applied.at(-1)).toBeNull();
  });

  it('an Auto source never asks the backend', async () => {
    const { correct, plan, translate, applied, sw } = setup({ from: 'auto' });
    await sw.translateWithSwitch();
    expect(correct).not.toHaveBeenCalled();
    expect(plan).not.toHaveBeenCalled();
    expect(translate).toHaveBeenCalledTimes(1);
    expect(applied.at(-1)).toBeNull();
  });

  it('an unchanged answer applies nothing and translates the original', async () => {
    const { plan, translate, applied, sw } = setup({ answer: NOTHING });
    await sw.translateWithSwitch();
    expect(applied.at(-1)).toBeNull();
    expect(plan).toHaveBeenCalledWith(expect.objectContaining({ text: ORIGINAL }));
    expect(translate).toHaveBeenCalledTimes(1);
  });

  it('a backend that fails is reported, applies nothing, and the text is translated as it came', async () => {
    const { plan, translate, applied, errors, busy, sw } = setup({ answer: new Error('boom') });
    await sw.translateWithSwitch();
    expect(errors).toHaveLength(1);
    expect(applied.at(-1)).toBeNull();
    expect(plan).toHaveBeenCalledWith(expect.objectContaining({ text: ORIGINAL }));
    expect(translate).toHaveBeenCalledTimes(1);
    expect(busy).toEqual([true, false]);
  });

  it('an answer for another text than the one on screen is not applied', async () => {
    const { applied, sw } = setup({
      answer: { ...CORRECTED, original: 'algo distinto por completo' },
    });
    await sw.translateWithSwitch();
    expect(applied.at(-1)).toBeNull();
  });

  it('a text edited while the model works is left alone', async () => {
    const { state, applied, translate, sw } = setup();
    let release: (a: CorrectionAnswer) => void = () => {};
    const slow = vi.fn(
      () =>
        new Promise<CorrectionAnswer>((r) => {
          release = r;
        }),
    );
    const s2 = createSourceSwitcher({
      plan: async () => NO_SWITCH,
      getText: () => state.text,
      getPair: () => ({ from: state.from, to: state.to }),
      setPair: () => {},
      autoCode: 'auto',
      sourceOptions: () => ['auto', 'en', 'es-MX'],
      canBeTarget: () => true,
      translate,
      onCue: () => {},
      correct: slow,
      isCorrectionEnabled: () => true,
      onCorrection: (c) => applied.push(c),
    });
    const done = s2.translateWithSwitch();
    await Promise.resolve();
    state.text = 'el usuario ya escribió otra cosa distinta';
    release(CORRECTED);
    await done;
    expect(applied).not.toContainEqual(expect.objectContaining({ text: FIXED }));
  });
});

describe('translating the original instead', () => {
  it('drops the correction and does not correct that text again', async () => {
    const { applied, correct, sw } = setup();
    await sw.translateWithSwitch();
    expect(applied.at(-1)).not.toBeNull();
    sw.useOriginal();
    expect(applied.at(-1)).toBeNull();
    await sw.translateWithSwitch();
    expect(correct).toHaveBeenCalledTimes(1);
    expect(applied.at(-1)).toBeNull();
  });

  it('a new arrival of the same text is corrected again', async () => {
    const { applied, sw } = setup();
    await sw.translateWithSwitch();
    sw.useOriginal();
    expect(applied.at(-1)).toBeNull();
    sw.newArrival();
    await sw.translateWithSwitch();
    expect(applied.at(-1)).not.toBeNull();
  });

  it('switching the checkbox forgets what was remembered', async () => {
    const { state, correct, sw } = setup();
    await sw.translateWithSwitch();
    state.enabled = false;
    sw.resetCorrection();
    await sw.translateWithSwitch();
    state.enabled = true;
    sw.resetCorrection();
    await sw.translateWithSwitch();
    expect(correct).toHaveBeenCalledTimes(2);
  });
});

describe('what the correction never does', () => {
  it('writes nothing to storage: no default, no variant taught', async () => {
    const { sw } = setup();
    await sw.translateWithSwitch();
    await sw.translateIfSwitched(ORIGINAL);
    sw.useOriginal();
    expect(writes).toEqual([]);
    expect(localStorage.length).toBe(0);
  });
});
