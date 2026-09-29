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
}

export function createSourceSwitcher(deps: SourceSwitchDeps) {
  let cue: SwitchCue | null = null;
  let declined = '';

  function setCue(next: SwitchCue | null) {
    cue = next;
    deps.onCue(next);
  }

  /** Switches the pair for `text` when the backend says so; returns whether it did. */
  async function autoSwitch(text: string, detected = ''): Promise<boolean> {
    if (deps.getPair().from === deps.autoCode || isDeclined(declined, text)) return false;
    const prev = { ...deps.getPair() };
    let plan: SourceSwitchPlan;
    try {
      plan = await deps.plan({ text, from: prev.from, to: prev.to, detected });
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
