// The undo/redo history of the translate window's source text (issue #118).
//
// The source pane had no dependable undo. The browser's own undo of the textarea was lost whenever
// the pane left edit mode (the textarea is destroyed for the SpanText view), and Clear, swap and a
// hotkey or clipboard fill wrote the text directly, so the native undo never saw them. Kai now owns
// the undo of the source text completely: every change is recorded here, and the window's Undo and
// Redo (the buttons, Cmd/Ctrl+Z, Shift+Cmd/Ctrl+Z, Ctrl+Y) walk back and forth through it.
//
// This module is the pure half: the history value and total functions over it. Every function
// returns a new value and mutates nothing, so assigning the result to a Svelte $state is the only
// reactivity needed. Pure function of plain values, like translateSession.ts / translateProgress.ts:
// no bindings import, no DOM, no clock (the caller passes `now`). TranslateWindow.svelte owns the
// wiring: one writer of the text (setSource) records every change, and applyUndo / applyRedo are
// the only other writers.
//
// Steps. A step is a snapshot of the whole text before a change:
// - typing coalesces: a run of typing records is one step. The run ends at the first pause of
//   coalesceMs or more (a sliding window, so "type a sentence, pause" is one step), at breakTyping
//   (the textarea lost focus), and at any paste or program change, undo or redo;
// - compose (an IME composition) continues an open run whatever the pause, so a pause inside a
//   composition never leaves a half-composed step; with no run open it starts one, like typing;
// - paste (also cut, drag and drop) and program (Clear, swap, a fill) are always a step of their
//   own and end the run;
// - a change that leaves the text as it was records nothing (Clear on an empty pane, a swap with
//   no result on screen, a fill with the same text), and keeps redo.
// Any recorded change clears redo.
//
// Caps. At most maxSteps undo steps and at most maxChars characters in the two stacks together; the
// oldest undo entries are dropped first, but the newest one is always kept, so Clear of a document
// larger than the cap can still be undone. The caps apply when a change is recorded. Undo and redo
// only move texts between the stacks and the text on screen, which the caller holds anyway: they
// never create a text, so what is held in memory (both stacks plus the text on screen) does not
// change and they drop nothing. Until the next record, `chars` can therefore exceed maxChars by the
// length of the text that was on screen.
//
// Snapshots of whole texts rather than diffs: the simplest correct model. JS strings are immutable,
// so an entry shared with the text on screen costs nothing extra, and the character cap bounds the
// worst case (100 steps of a 100,000-character document would be 10,000,000 characters; the cap
// keeps 20 of them).

/** What changed the source text; decides how the change is grouped into undo steps. */
export type ChangeKind = 'typing' | 'compose' | 'paste' | 'program';

/** The bounds of the history and its typing window. */
export interface HistoryLimits {
  /** The most undo steps kept; the oldest go first. */
  maxSteps: number;
  /** The most characters held in the undo and redo stacks together (the newest entry is kept). */
  maxChars: number;
  /** A pause of this many ms or more between two typing records starts a new step. */
  coalesceMs: number;
}

/** 100 steps, 2,000,000 characters, a 1 s typing window. */
export const DEFAULT_LIMITS: HistoryLimits = {
  maxSteps: 100,
  maxChars: 2_000_000,
  coalesceMs: 1000,
};

/** The history of the source text. Never mutated: every function returns a new value. */
export interface SourceHistory {
  /** The text before each recorded step, oldest first; undo takes the last one. */
  readonly undo: readonly string[];
  /** The texts undone, most recent last; redo takes the last one. Any new record clears it. */
  readonly redo: readonly string[];
  /** The total length of the undo and redo entries. */
  readonly chars: number;
  /** The time of the last typing or compose record while a run is open; null when none is. */
  readonly typingAt: number | null;
}

/** The empty history: nothing to undo or redo, no typing run open. A fresh value on every call. */
export function emptyHistory(): SourceHistory {
  return { undo: [], redo: [], chars: 0, typingAt: null };
}

// The summed length of a list of texts.
function lengthOf(texts: readonly string[]): number {
  let n = 0;
  for (const text of texts) n += text.length;
  return n;
}

// The history after a push onto undo, with the caps applied: the oldest undo entry is dropped
// while there are more than maxSteps entries, or while more than maxChars characters are held and
// more than one entry is left (the newest entry is always kept). Redo is empty: the record that
// pushed has just cleared it. `undo` is the caller's new array; it is sliced, never changed.
function pushed(
  undo: string[],
  chars: number,
  typingAt: number | null,
  limits: HistoryLimits,
): SourceHistory {
  let drop = 0;
  let held = chars;
  while (
    drop < undo.length &&
    (undo.length - drop > limits.maxSteps || (held > limits.maxChars && undo.length - drop > 1))
  ) {
    held -= undo[drop].length;
    drop += 1;
  }
  return { undo: drop > 0 ? undo.slice(drop) : undo, redo: [], chars: held, typingAt };
}

/**
 * Records a change of the source text and returns the new history.
 *
 * - prev === next: nothing changed; the same history comes back (no step, redo kept).
 * - Otherwise redo is cleared, and by kind:
 *   - typing: inside an open run (typingAt set) and less than coalesceMs after its last record,
 *     nothing is pushed and the run slides on (typingAt = now); otherwise prev is pushed and a run
 *     opens at now;
 *   - compose: like typing, but an open run is never ended by a pause;
 *   - paste, program: prev is always pushed and the run ends.
 * - After a push the caps apply (see pushed above).
 *
 * @param h the history before the change
 * @param prev the text before the change
 * @param next the text after the change
 * @param kind what made the change
 * @param now the caller's clock, in ms (only differences are used)
 * @param limits the caps and the typing window; DEFAULT_LIMITS when omitted
 */
export function record(
  h: SourceHistory,
  prev: string,
  next: string,
  kind: ChangeKind,
  now: number,
  limits: HistoryLimits = DEFAULT_LIMITS,
): SourceHistory {
  if (prev === next) return h;
  // A real change clears redo, so the redo entries no longer count.
  const chars = h.chars - lengthOf(h.redo);
  if (kind === 'typing' || kind === 'compose') {
    const at = h.typingAt;
    if (at !== null && (kind === 'compose' || now - at < limits.coalesceMs)) {
      return { undo: h.undo, redo: [], chars, typingAt: now };
    }
    return pushed([...h.undo, prev], chars + prev.length, now, limits);
  }
  return pushed([...h.undo, prev], chars + prev.length, null, limits);
}

/** Ends the typing run (the textarea lost focus): the next typing record starts a new step. */
export function breakTyping(h: SourceHistory): SourceHistory {
  return h.typingAt === null ? h : { ...h, typingAt: null };
}

/**
 * Undoes the last step: returns the text to show (the last undo entry) and the history with that
 * entry taken off undo and `current` put on redo, the typing run ended. null when there is nothing
 * to undo.
 *
 * @param current the text on screen now; redo brings it back
 */
export function undo(
  h: SourceHistory,
  current: string,
): { history: SourceHistory; text: string } | null {
  const last = h.undo.length - 1;
  if (last < 0) return null;
  const text = h.undo[last];
  return {
    history: {
      undo: h.undo.slice(0, last),
      redo: [...h.redo, current],
      chars: h.chars - text.length + current.length,
      typingAt: null,
    },
    text,
  };
}

/**
 * Redoes the last undone step, the mirror of undo: returns the text to show (the last redo entry)
 * and the history with that entry taken off redo and `current` put back on undo, the typing run
 * ended. null when there is nothing to redo.
 *
 * @param current the text on screen now; undo brings it back
 */
export function redo(
  h: SourceHistory,
  current: string,
): { history: SourceHistory; text: string } | null {
  const last = h.redo.length - 1;
  if (last < 0) return null;
  const text = h.redo[last];
  return {
    history: {
      undo: [...h.undo, current],
      redo: h.redo.slice(0, last),
      chars: h.chars - text.length + current.length,
      typingAt: null,
    },
    text,
  };
}

/** Whether there is a step to undo (the Undo button is disabled otherwise). */
export function canUndo(h: SourceHistory): boolean {
  return h.undo.length > 0;
}

/** Whether there is a step to redo (the Redo button is disabled otherwise). */
export function canRedo(h: SourceHistory): boolean {
  return h.redo.length > 0;
}
