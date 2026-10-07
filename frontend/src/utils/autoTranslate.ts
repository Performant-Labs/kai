// The frontend half of "translate as I type" (issue #57).
//
// With the switch on, text the user types or pastes into the source box is translated without a
// click. The translation itself is the Translate button's own path (the automatic source switch, the
// correction and the back-translation apply as they do for a click). What lives here is the small
// pure part, so vitest covers it without the window: which edits start a translation and after how
// long, the timer that starts it, and whether the history write that follows a quiet moment still
// describes the text on screen.
//
// Nothing here persists.

/** How long the user must stop typing before the text is translated. Short enough to feel live, long enough to skip most keystrokes. */
export const AUTO_TYPING_DELAY_MS = 600;
/** Typed text shorter than this (after trimming) does not start a translation: one or two letters are not a request. */
export const AUTO_MIN_TYPED_CHARS = 3;
/** How long the text must stay unchanged after an automatic translation before it is written to the history. */
export const AUTO_COMMIT_DELAY_MS = 2500;

/** What an edit of the source text was: typing, a paste or drop, an IME composition, or a programmatic fill. */
export type AutoKind = 'typing' | 'paste' | 'compose' | 'program';

/**
 * The kind of an edit, read off its input event. A paste or a drop is one finished action. A cut or
 * a drag-out is typing: the text that is left is translated after a pause. An IME word that is still
 * being composed is composing: it is not final, so nothing translates it. The commit of a composed
 * word is typing again: WebKit reports it AFTER compositionend, as deleteCompositionText and
 * insertFromComposition with isComposing false, so those must start a pause, not cancel the wait
 * compositionend started.
 */
export function autoKindOf(inputType: string, isComposing: boolean): AutoKind {
  if (inputType === 'insertFromPaste' || inputType === 'insertFromDrop') return 'paste';
  if (isComposing || inputType === 'insertCompositionText') return 'compose';
  return 'typing';
}

/**
 * How long to wait before translating this edit, or null for "do not translate by itself". Null with
 * the switch off, with no engine to translate with, for blank text, for a few typed characters, for
 * a half-composed IME word, and for a programmatic fill (the hotkey, the tray and the clipboard
 * translate on their own). A paste is translated at once.
 */
export function autoTranslateDelay(i: {
  enabled: boolean;
  text: string;
  kind: AutoKind;
  hasEngines: boolean;
}): number | null {
  if (!i.enabled || !i.hasEngines) return null;
  const trimmed = i.text.trim();
  if (trimmed === '') return null;
  switch (i.kind) {
    case 'paste':
      return 0;
    case 'typing':
      return Array.from(trimmed).length >= AUTO_MIN_TYPED_CHARS ? AUTO_TYPING_DELAY_MS : null;
    default:
      return null;
  }
}

export interface AutoScheduler {
  /** Run after `ms` (at once for 0), replacing any wait that was pending. */
  schedule(ms: number): void;
  cancel(): void;
  pending(): boolean;
}

/** A restartable one-shot timer: each schedule replaces the previous wait, so a burst of edits runs once. */
export function createAutoScheduler(
  run: () => void,
  timers: {
    set: (fn: () => void, ms: number) => unknown;
    clear: (id: unknown) => void;
  } = {
    set: (fn, ms) => setTimeout(fn, ms),
    clear: (id) => clearTimeout(id as ReturnType<typeof setTimeout>),
  },
): AutoScheduler {
  let id: unknown;
  let waiting = false;
  const cancel = () => {
    if (waiting) timers.clear(id);
    waiting = false;
  };
  return {
    schedule(ms: number) {
      cancel();
      if (ms <= 0) {
        run();
        return;
      }
      waiting = true;
      id = timers.set(() => {
        waiting = false;
        run();
      }, ms);
    },
    cancel,
    pending: () => waiting,
  };
}

/**
 * Whether the history write that follows a quiet moment is still right: the text on screen is the
 * one the automatic request was made for, that request is still the window's current one, and the
 * text is not blank.
 */
export function commitStillValid(i: {
  requestText: string;
  currentText: string;
  current: boolean;
}): boolean {
  return i.current && i.requestText.trim() !== '' && i.requestText === i.currentText;
}
