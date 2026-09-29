import { describe, expect, it } from 'vitest';
import {
  acceptSwitch,
  cueActive,
  isDeclined,
  makeCue,
  undoPair,
  type SourceSwitchPlan,
} from './sourceSwitch.ts';

// Issue #200: the frontend half of the automatic source switch is a thin, pure gate. The backend
// (translate.Service.PlanSourceSwitch) decides; this only refuses a plan the dropdowns cannot hold
// and keeps the undo cue. No bindings import, like swapLangs.ts.

const current = { from: 'en', to: 'fr' };
const plan: SourceSwitchPlan = { switched: true, from: 'es-MX', to: 'en' };
const ctx = {
  current,
  autoCode: 'auto',
  sourceOptions: ['auto', 'en', 'fr', 'es-MX'],
  isSelectableTarget: (c: string) => ['en', 'fr', 'es-MX'].includes(c),
};

describe('acceptSwitch', () => {
  it('returns the plan as a pair when both dropdowns can hold it', () => {
    expect(acceptSwitch(plan, ctx)).toEqual({ from: 'es-MX', to: 'en' });
  });

  it('refuses a missing, unswitched or null plan', () => {
    expect(acceptSwitch(null, ctx)).toBeNull();
    expect(acceptSwitch(undefined, ctx)).toBeNull();
    expect(acceptSwitch({ switched: false, from: '', to: '' }, ctx)).toBeNull();
  });

  it('never switches an Auto source', () => {
    expect(acceptSwitch(plan, { ...ctx, current: { from: 'auto', to: 'fr' } })).toBeNull();
  });

  it('refuses when the new source is not a source option', () => {
    expect(acceptSwitch({ ...plan, from: 'ja' }, ctx)).toBeNull();
  });

  it('refuses when the old source cannot be the target (missing or capability-disabled)', () => {
    expect(acceptSwitch(plan, { ...ctx, isSelectableTarget: () => false })).toBeNull();
  });

  it('refuses a plan whose target is not the old source (the old target is replaced by it)', () => {
    expect(acceptSwitch({ ...plan, to: 'fr' }, ctx)).toBeNull();
  });

  it('refuses a plan that would leave both sides the same language', () => {
    expect(acceptSwitch({ switched: true, from: 'en', to: 'en' }, ctx)).toBeNull();
  });
});

describe('undo cue', () => {
  const cue = makeCue('Hola', current, { from: 'es-MX', to: 'en' });

  it('is active only for the same text and the pair the switch produced', () => {
    expect(cueActive(cue, 'Hola', { from: 'es-MX', to: 'en' })).toBe(true);
    expect(cueActive(cue, 'Hola!', { from: 'es-MX', to: 'en' })).toBe(false);
    expect(cueActive(cue, 'Hola', { from: 'es-MX', to: 'fr' })).toBe(false);
    expect(cueActive(cue, 'Hola', current)).toBe(false);
    expect(cueActive(null, 'Hola', current)).toBe(false);
  });

  it('undoPair restores the previous pair exactly (not a plain swap of the new one)', () => {
    expect(undoPair(cue, 'Hola', { from: 'es-MX', to: 'en' })).toEqual({ from: 'en', to: 'fr' });
  });

  it('undoPair is null once the cue no longer applies, so the ordinary swap runs', () => {
    expect(undoPair(cue, 'Hola', { from: 'ja', to: 'en' })).toBeNull();
    expect(undoPair(null, 'Hola', current)).toBeNull();
  });
});

describe('isDeclined', () => {
  it('matches the text the user undid, ignoring surrounding whitespace', () => {
    expect(isDeclined('Hola amigo', ' Hola amigo\n')).toBe(true);
    expect(isDeclined('Hola amigo', 'Hola amigo mio')).toBe(false);
  });
  it('an empty declined text declines nothing', () => {
    expect(isDeclined('', '')).toBe(false);
    expect(isDeclined('', 'x')).toBe(false);
  });
});
