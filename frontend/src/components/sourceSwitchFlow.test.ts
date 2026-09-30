import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createSourceSwitcher, type SourceSwitchDeps } from './sourceSwitchFlow.ts';
import type { SourceSwitchPlan, SwitchCue } from '../utils/sourceSwitch.ts';

// Issue #200: the switch flow is EXECUTED here, with fakes for only the Wails boundary (the planner
// binding) and the translate callback. The planner's decisions (length floor, confidence, dialects,
// the setting) are the backend's and are tested in Go (internal/translate/source_switch_test.go);
// here the planner answers what it is told to, and the tests pin what the window does with it.
// localStorage is the real jsdom one: an automatic switch must not write to it.

const SPANISH = 'Hola, necesito que me ayudes con este documento hoy';
const SWITCH: SourceSwitchPlan = { switched: true, from: 'es-MX', to: 'en' };
const NONE: SourceSwitchPlan = { switched: false, from: '', to: '' };

function setup(opts: { plan?: SourceSwitchPlan; from?: string; to?: string; text?: string } = {}) {
  const skips: string[] = [];
  const state = { text: opts.text ?? SPANISH, from: opts.from ?? 'en', to: opts.to ?? 'fr' };
  const cues: (SwitchCue | null)[] = [];
  const plan = vi.fn(async () => opts.plan ?? SWITCH);
  const translate = vi.fn();
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
    onCue: (c) => cues.push(c),
    onSkip: (reason) => skips.push(reason),
  };
  return { state, cues, skips, plan, translate, sw: createSourceSwitcher(deps) };
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

describe('a mismatching arrival', () => {
  it('switches the source, replaces the target with the old source, and translates once', async () => {
    const { state, translate, plan, sw } = setup();
    sw.newArrival();
    await sw.translateWithSwitch();
    expect(plan).toHaveBeenCalledWith({ text: SPANISH, from: 'en', to: 'fr', detected: '' });
    expect(state).toMatchObject({ from: 'es-MX', to: 'en' });
    expect(translate).toHaveBeenCalledTimes(1);
  });

  it('shows a cue for the pair it made', async () => {
    const { cues, sw } = setup();
    await sw.translateWithSwitch();
    expect(cues.at(-1)).toMatchObject({
      prev: { from: 'en', to: 'fr' },
      next: { from: 'es-MX', to: 'en' },
    });
  });

  it('writes nothing to storage: it neither persists the pair nor teaches a variant', async () => {
    const { sw } = setup();
    await sw.translateWithSwitch();
    await sw.translateIfSwitched(SPANISH);
    expect(writes).toEqual([]);
    expect(localStorage.length).toBe(0);
  });
});

describe('nothing to switch', () => {
  it('a match (the planner says no) changes nothing and still translates through Translate', async () => {
    const { state, translate, cues, sw } = setup({ plan: NONE });
    await sw.translateWithSwitch();
    expect(state).toMatchObject({ from: 'en', to: 'fr' });
    expect(cues).toEqual([]);
    expect(translate).toHaveBeenCalledTimes(1);
  });

  it('short text, low confidence and the setting off are all the planner answering no: same outcome', async () => {
    for (const text of ['Hola', 'x'.repeat(40)]) {
      const { state, translate, sw } = setup({ plan: NONE, text });
      await sw.translateWithSwitch();
      expect(state).toMatchObject({ from: 'en', to: 'fr' });
      expect(translate).toHaveBeenCalledTimes(1);
    }
  });

  it('an Auto source never asks the planner and never switches', async () => {
    const { state, plan, translate, sw } = setup({ from: 'auto' });
    await sw.translateWithSwitch();
    expect(plan).not.toHaveBeenCalled();
    expect(state.from).toBe('auto');
    expect(translate).toHaveBeenCalledTimes(1);
  });

  it('a plan the selects cannot hold is refused (target disabled, unknown source, wrong old source)', async () => {
    for (const plan of [
      { switched: true, from: 'ja', to: 'en' },
      { switched: true, from: 'es-MX', to: 'fr' },
    ]) {
      const { state, sw } = setup({ plan });
      await sw.translateWithSwitch();
      expect(state).toMatchObject({ from: 'en', to: 'fr' });
    }
  });

  it('a failing planner leaves the pair alone and still translates', async () => {
    const { state, translate, sw, plan } = setup();
    plan.mockRejectedValueOnce(new Error('boom'));
    await sw.translateWithSwitch();
    expect(state).toMatchObject({ from: 'en', to: 'fr' });
    expect(translate).toHaveBeenCalledTimes(1);
  });

  it('an answer that arrives after the text changed is dropped', async () => {
    const { state, sw, plan } = setup();
    plan.mockImplementationOnce(async () => {
      state.text = 'something else entirely';
      return SWITCH;
    });
    expect(await sw.autoSwitch(SPANISH)).toBe(false);
    expect(state).toMatchObject({ from: 'en', to: 'fr' });
  });
});

describe('undo', () => {
  it('restores the previous pair exactly, keeps the text, translates, and clears the cue', async () => {
    const { state, cues, translate, sw } = setup();
    await sw.translateWithSwitch();
    translate.mockClear();
    expect(sw.undo()).toBe(true);
    expect(state).toMatchObject({ from: 'en', to: 'fr', text: SPANISH });
    expect(translate).toHaveBeenCalledTimes(1);
    expect(cues.at(-1)).toBeNull();
    expect(writes).toEqual([]);
  });

  it('the same text is not switched again after an undo, but a new arrival is', async () => {
    const { state, sw, plan } = setup();
    await sw.translateWithSwitch();
    sw.undo();
    plan.mockClear();
    await sw.translateWithSwitch();
    expect(plan).not.toHaveBeenCalled();
    expect(state).toMatchObject({ from: 'en', to: 'fr' });
    sw.newArrival();
    await sw.translateWithSwitch();
    expect(state).toMatchObject({ from: 'es-MX', to: 'en' });
  });

  it('is nothing once the user changed a select or the text: the ordinary swap runs', async () => {
    const { state, sw } = setup();
    await sw.translateWithSwitch();
    state.to = 'fr';
    expect(sw.undo()).toBe(false);
    state.to = 'en';
    state.text = 'other';
    expect(sw.undo()).toBe(false);
  });
});

describe('paste and the engine-detection retry', () => {
  it('translates only if it switched', async () => {
    const yes = setup();
    await yes.sw.translateIfSwitched(SPANISH);
    expect(yes.translate).toHaveBeenCalledTimes(1);
    const no = setup({ plan: NONE });
    await no.sw.translateIfSwitched(SPANISH);
    expect(no.translate).not.toHaveBeenCalled();
  });

  it('passes a detection the caller holds on to the planner', async () => {
    const { plan, sw } = setup();
    await sw.translateIfSwitched(SPANISH, 'es-MX');
    expect(plan).toHaveBeenCalledWith({ text: SPANISH, from: 'en', to: 'fr', detected: 'es-MX' });
  });
});

describe('typed text on Translate', () => {
  it('is checked when Translate is pressed, and switches', async () => {
    const { state, translate, sw } = setup({ text: SPANISH });
    // typing changed nothing until now
    expect(state.from).toBe('en');
    await sw.translateWithSwitch();
    expect(state).toMatchObject({ from: 'es-MX', to: 'en' });
    expect(translate).toHaveBeenCalledTimes(1);
  });
});

// Issue #16: every way the flow ends without a switch names its reason, so a miss is never silent.
// The backend's own reason passes through when it said no; the frontend's own drops say theirs.
describe('the reason of a miss', () => {
  it('passes the backend reason through when the planner says no', async () => {
    for (const reason of [
      'below_confidence',
      'too_short',
      'same_language',
      'no_detection',
      'disabled',
    ]) {
      const { skips, sw } = setup({ plan: { ...NONE, reason } });
      await sw.translateWithSwitch();
      expect(skips).toEqual([reason]);
    }
  });

  it('says source_auto for an Auto source', async () => {
    const { skips, sw } = setup({ from: 'auto' });
    await sw.translateWithSwitch();
    expect(skips).toEqual(['source_auto']);
  });

  it('says declined for a text whose switch the user undid', async () => {
    const { skips, sw, plan } = setup();
    await sw.translateWithSwitch();
    sw.undo();
    skips.length = 0;
    await sw.translateWithSwitch();
    expect(skips).toEqual(['declined']);
    expect(plan).toHaveBeenCalledTimes(1);
  });

  it('says text_changed when the text moved while the plan was computed', async () => {
    const { state, skips, sw, plan } = setup();
    plan.mockImplementationOnce(async () => {
      state.text = 'something else entirely';
      return SWITCH;
    });
    await sw.autoSwitch(SPANISH);
    expect(skips).toEqual(['text_changed']);
  });

  it('says pair_changed when a select moved while the plan was computed', async () => {
    const { state, skips, sw, plan } = setup();
    plan.mockImplementationOnce(async () => {
      state.to = 'es-MX';
      return SWITCH;
    });
    await sw.autoSwitch(SPANISH);
    expect(skips).toEqual(['pair_changed']);
  });

  it('says plan_failed when the planner throws', async () => {
    const { skips, sw, plan } = setup();
    plan.mockRejectedValueOnce(new Error('boom'));
    await sw.autoSwitch(SPANISH);
    expect(skips).toEqual(['plan_failed']);
  });

  it('names why a plan the selects cannot hold was refused', async () => {
    const cases: [SourceSwitchPlan, string][] = [
      [{ switched: true, from: 'ja', to: 'en' }, 'source_not_offered'],
      [{ switched: true, from: 'es-MX', to: 'fr' }, 'plan_mismatch'],
    ];
    for (const [plan, reason] of cases) {
      const { skips, sw } = setup({ plan });
      await sw.autoSwitch(SPANISH);
      expect(skips).toEqual([reason]);
    }
  });

  it('says nothing when it switched', async () => {
    const { skips, sw } = setup();
    await sw.autoSwitch(SPANISH);
    expect(skips).toEqual([]);
  });
});
