// The flow of the automatic source switch (issue #200), out of TranslateWindow.svelte so vitest can
// run it: every dependency comes in as an argument, and the only things a test fakes are the
// Wails binding boundary (the planner call) and the translate callback.
//
// Every way text arrives goes through the one autoSwitch: the window's fill, Translate on typed
// text, a paste, a result that still carries the engine's detection, and the double Cmd+C trigger
// (#199) later. The DECISION is the backend's (PlanSourceSwitch); this applies its answer to the two
// selects when they can hold it. It has no access to the variant store or to persistence, by
// construction: the only writes are `setPair`, `onCue` and `translate`, all supplied by the caller,
// and the caller supplies none that persist or teach.

import {
  acceptCorrection,
  type AppliedCorrection,
  type CorrectionAnswer,
} from '../utils/sourceCorrection.ts';
import {
  acceptSwitch,
  isDeclined,
  makeCue,
  undoPair,
  type SourceSwitchPlan,
  type SwitchCue,
} from '../utils/sourceSwitch.ts';
import type { LangPair } from '../utils/swapLangs.ts';

export interface SourceSwitchRequest {
  text: string;
  from: string;
  to: string;
  detected: string;
}

export interface CorrectionRequest {
  text: string;
  from: string;
  detected: string;
}

export interface SourceSwitchDeps {
  /** The planner (translate.Service.PlanSourceSwitch through the binding). */
  plan: (req: SourceSwitchRequest) => Promise<SourceSwitchPlan>;
  /** The source text on screen now. */
  getText: () => string;
  /** The pair the selects show now. */
  getPair: () => LangPair;
  /** Assigns the two selects. Must not persist or teach. */
  setPair: (pair: LangPair) => void;
  /** The language code for auto. */
  autoCode: string;
  /** The source select's option codes. */
  sourceOptions: () => readonly string[];
  /** Whether the target select can hold a code. */
  canBeTarget: (code: string) => boolean;
  /** Runs the translation for the current text and pair. */
  translate: () => unknown;
  /** Told when the cue changes (null: none). */
  onCue: (cue: SwitchCue | null) => void;
  onError?: (e: unknown) => void;
  /**
   * The correction step (issue #208), optional: with `correct` and `isCorrectionEnabled` given, every
   * arrival is corrected FIRST (translate.Service.CorrectSource through the binding), and the
   * switch planner and the translation go on with the corrected text. The backend decides
   * everything (setting, language, length, guards, diff); this only calls it, applies its answer
   * through `onCorrection`, and never persists or teaches anything.
   */
  correct?: (req: CorrectionRequest) => Promise<CorrectionAnswer>;
  /** Whether the checkbox is on (and the provider available); off never asks the backend. */
  isCorrectionEnabled?: () => boolean;
  /** Told the correction that applies to the arrival (null: none). */
  onCorrection?: (c: AppliedCorrection | null) => void;
  /** Told when the model starts and stops working, so the window can show it. */
  onCorrecting?: (busy: boolean) => void;
}

export function createSourceSwitcher(deps: SourceSwitchDeps) {
  let cue: SwitchCue | null = null;
  let declined = '';
  // The text whose correction the user turned down ("translate the original"): not corrected again
  // until a new arrival. And the last answer, so a paste followed by Translate, or the engine
  // detection retry, does not run the model twice for the same text.
  let correctionDeclined = '';
  let memo: { key: string; answer: CorrectionAnswer } | null = null;

  const memoKey = (from: string, text: string) => `${from}\u0000${text}`;

  function setCorrection(c: AppliedCorrection | null) {
    deps.onCorrection?.(c);
  }

  /**
   * The text the rest of the arrival works on: the corrected text when the backend corrected it,
   * else the text as it came. Records the correction with the window either way, so a stale one
   * never survives into an arrival that has none.
   */
  async function workingText(text: string, detected: string): Promise<string> {
    const from = deps.getPair().from;
    // (An Auto source never gets here: autoSwitch returns first, and clears the correction.)
    const off = !deps.isCorrectionEnabled?.() || correctionDeclined.trim() === text.trim();
    if (off || !deps.correct) {
      setCorrection(null);
      return text;
    }
    const key = memoKey(from, text);
    let answer: CorrectionAnswer | null = memo?.key === key ? memo.answer : null;
    if (!answer) {
      deps.onCorrecting?.(true);
      try {
        answer = await deps.correct({ text, from, detected });
      } catch (e) {
        deps.onError?.(e);
        answer = null;
      } finally {
        deps.onCorrecting?.(false);
      }
      // Only an answer that is a fact about the text is remembered: an "off" or "unavailable" one
      // must be asked again once the setting or the model changes.
      if (answer && (answer.status === 'corrected' || answer.status === 'unchanged')) {
        memo = { key, answer };
      }
    }
    // The user moved on while the model worked: this answer is not for the text on screen.
    if (deps.getText() !== text) return text;
    const applied = acceptCorrection(answer, text);
    setCorrection(applied);
    return applied ? applied.text : text;
  }

  function setCue(next: SwitchCue | null) {
    cue = next;
    deps.onCue(next);
  }

  /** Switches the pair for `text` when the backend says so; returns whether it did. */
  async function autoSwitch(text: string, detected = ''): Promise<boolean> {
    if (deps.getPair().from === deps.autoCode) {
      setCorrection(null);
      return false;
    }
    // Correct first (issue #208); the switch is planned on the text that will be translated.
    const working = await workingText(text, detected);
    if (isDeclined(declined, text)) return false;
    const prev = { ...deps.getPair() };
    let plan: SourceSwitchPlan;
    try {
      plan = await deps.plan({ text: working, from: prev.from, to: prev.to, detected });
    } catch (e) {
      deps.onError?.(e);
      return false;
    }
    // The user moved on while the answer was on its way.
    const now = deps.getPair();
    if (deps.getText() !== text || now.from !== prev.from || now.to !== prev.to) return false;
    const pair = acceptSwitch(plan, {
      current: prev,
      autoCode: deps.autoCode,
      sourceOptions: deps.sourceOptions(),
      isSelectableTarget: deps.canBeTarget,
    });
    if (!pair) return false;
    // The switch changed the source the answer was asked under: it is still the answer for this text.
    if (memo?.key === memoKey(prev.from, text)) memo.key = memoKey(pair.from, text);
    setCue(makeCue(text, prev, pair));
    deps.setPair(pair);
    return true;
  }

  return {
    autoSwitch,
    /** Translate the source text, switching the pair first when the text is in another language. */
    async translateWithSwitch() {
      await autoSwitch(deps.getText());
      await deps.translate();
    },
    /** For an arrival that does not translate by itself: translate only when it switched. */
    async translateIfSwitched(text: string, detected = '') {
      if (await autoSwitch(text, detected)) await deps.translate();
    },
    /** A new text arrived: an undo of an earlier text's switch does not carry over. */
    newArrival() {
      declined = '';
      correctionDeclined = '';
    },
    /**
     * "Translate the original instead" (issue #208): drops the correction of the text on screen and
     * does not correct it again until a new arrival. The caller translates.
     */
    useOriginal() {
      correctionDeclined = deps.getText();
      setCorrection(null);
    },
    /** The checkbox was switched: what was remembered about earlier answers is forgotten. */
    resetCorrection() {
      memo = null;
      correctionDeclined = '';
    },
    /**
     * The swap button while the cue is active: restores the pair before the switch, keeps the text,
     * marks it so it is not switched again, and translates. False when there is nothing to undo.
     */
    undo(): boolean {
      const text = deps.getText();
      const pair = undoPair(cue, text, deps.getPair());
      if (!pair) return false;
      declined = text;
      deps.setPair(pair);
      setCue(null);
      deps.translate();
      return true;
    },
  };
}
