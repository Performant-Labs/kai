// The frontend half of the automatic source switch (issue #200).
//
// Text that arrives in another language than the pinned source dropdown shows switches the
// dropdown to that language, makes the old source the target, and translates. The DECISION is the
// backend's (translate.Service.PlanSourceSwitch: local detection, the 20-code-point floor, the
// confidence check, dialect matching, the setting): this window compares no languages and counts no
// characters. What lives here is the small pure gate between that answer and the two selects, and
// the undo cue, so vitest covers it without the bindings (like swapLangs.ts).
//
// Nothing here teaches the variant store or persists the pair: an automatic switch is not a choice
// the user made. Only a select's own onchange does that.

import type { LangPair } from './swapLangs.ts';

/** The backend's answer (model.SourceSwitch): the pair the window should take, when Switched. */
export interface SourceSwitchPlan {
  switched: boolean;
  from: string;
  to: string;
  /** Why (model.SourceSwitch.Reason, issue #16): "switched", or what kept it from switching. */
  reason?: string;
}

/** What acceptSwitch checks a plan against (all injected by the caller). */
export interface AcceptContext {
  /** The pair the window shows now. */
  current: LangPair;
  /** The language code for auto (TRANSLATE_LANG.Auto). */
  autoCode: string;
  /** The source select's option codes. */
  sourceOptions: readonly string[];
  /** Whether the target select can hold a code: offered, and not capability-disabled (#52). */
  isSelectableTarget: (code: string) => boolean;
}

/**
 * The pair to apply, or null when the plan must not be applied: nothing to switch, an Auto source
 * (it has no entry to switch), a new source the source select does not offer, an old source the
 * target select cannot hold, a plan whose target is not the old source (the old target is replaced
 * by the old source, always), or both sides ending up the same.
 */
export function acceptSwitch(
  plan: SourceSwitchPlan | null | undefined,
  ctx: AcceptContext,
): LangPair | null {
  return rejectReason(plan, ctx) === null && plan ? { from: plan.from, to: plan.to } : null;
}

/**
 * Why acceptSwitch refuses a plan (issue #16), null when it accepts it. When the planner itself
 * said no, its own reason is passed through ("no_switch" if it gave none).
 */
export function rejectReason(
  plan: SourceSwitchPlan | null | undefined,
  ctx: AcceptContext,
): string | null {
  if (!plan || !plan.switched) return plan?.reason || 'no_switch';
  if (ctx.current.from === ctx.autoCode) return 'source_auto';
  if (plan.to !== ctx.current.from || plan.from === plan.to) return 'plan_mismatch';
  if (!ctx.sourceOptions.includes(plan.from)) return 'source_not_offered';
  if (!ctx.isSelectableTarget(plan.to)) return 'target_not_selectable';
  return null;
}

/** The note that says the pair was switched. */
export interface SwitchCue {
  /** The source text the switch was made for. */
  text: string;
  /** The pair before the switch. */
  prev: LangPair;
  /** The pair the switch produced. */
  next: LangPair;
}

export function makeCue(text: string, prev: LangPair, next: LangPair): SwitchCue {
  return { text, prev: { ...prev }, next: { ...next } };
}

/**
 * Whether the cue still describes the window: the same text and still the pair the switch made.
 * Any later change of the text or of either select ends it, so the note never talks about a state
 * that is gone.
 */
export function cueActive(cue: SwitchCue | null, text: string, current: LangPair): boolean {
  return (
    cue !== null &&
    cue.text === text &&
    cue.next.from === current.from &&
    cue.next.to === current.to
  );
}

/**
 * The pair that undoes the switch while the cue is active (issue #39: the window's swap button no
 * longer calls this; it always swaps for real): the pair before the switch, exactly
 * (not a plain swap of the new one, which would leave the old source as target and lose the old
 * target). Null when the cue no longer applies, and the ordinary swap runs.
 */
export function undoPair(cue: SwitchCue | null, text: string, current: LangPair): LangPair | null {
  return cueActive(cue, text, current) ? { ...cue!.prev } : null;
}

/**
 * Whether `text` is the text whose switch the user undid. It is not switched again: pressing
 * Translate after an undo must not put the pair back the user just took back.
 */
export function isDeclined(declined: string, text: string): boolean {
  return declined.trim() !== '' && declined.trim() === text.trim();
}
