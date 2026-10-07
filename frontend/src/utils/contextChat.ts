// The frontend half of the result pane's context chat (issue #48).
//
// The user tells the translator it got the context wrong ("junta means a meeting here", "be
// formal") and the text is translated again in that context. The backend
// (translate.Service.RetranslateWithContext) decides everything that matters: which engine answers
// (the selected one when it can follow a context, else the on-device model, else a configured cloud
// LLM), the prompt, the guards. What lives here is the small pure state between the backend's answers
// and the window, so vitest covers it without the bindings (like sourceCorrection.ts): the context
// the user has given (kept across texts until cleared), the request, what an answer means for the
// window, and the clear shortcut.
//
// Nothing here persists. The context is session state of the window; clearChat is the one way out.

/** One line of the chat. A `note` is the window talking (why nothing could be done). */
export interface ChatMessage {
  role: 'user' | 'assistant' | 'note';
  text: string;
  /** The engine that produced an assistant line. */
  engine?: string;
}

/** The chat: what the user told the translator (oldest first) and the lines shown. */
export interface ContextChat {
  context: string[];
  messages: ChatMessage[];
}

export const emptyChat: ContextChat = { context: [], messages: [] };

/** The backend's answer (model.ContextTranslateResult). */
export interface ContextAnswer {
  status: 'ok' | 'cancelled' | 'unavailable' | 'failed' | 'no_context';
  result: string;
  engine: string;
  fallback: boolean;
  reason: string;
  error: string;
  request_id: string;
}

/** The retranslation the result pane shows in place of the engine's own result. */
export interface ShownRetranslation {
  text: string;
  /** The engine that produced it (not always the one the pane is bound to: see `fallback`). */
  engine: string;
  /** True when the selected engine could not follow a context and another one did. */
  fallback: boolean;
}

/** A retranslation, remembered with the text and the engine it was made for. */
export interface BoundRetranslation extends ShownRetranslation {
  forInput: string;
  forEngine: string;
}

export function hasContext(c: ContextChat): boolean {
  return c.context.length > 0;
}

/** The chat with what the user just said added to the context. A blank message changes nothing. */
export function addUserMessage(c: ContextChat, text: string): ContextChat {
  const t = text.trim();
  if (t === '') return c;
  return {
    context: [...c.context, t],
    messages: [...c.messages, { role: 'user', text: t }],
  };
}

/** The chat after "clear context": no context and no history. */
export function clearChat(): ContextChat {
  return { context: [], messages: [] };
}

/** The request RetranslateWithContext takes (model.ContextTranslateRequest). */
export function buildContextRequest(
  c: ContextChat,
  r: {
    text: string;
    from: string;
    to: string;
    engine: string;
    previous: string;
    requestId: string;
  },
) {
  return {
    text: r.text,
    from: r.from,
    to: r.to,
    engine: r.engine,
    previous: r.previous,
    context: [...c.context],
    request_id: r.requestId,
  };
}

/** The i18n key that words an answer the user must be told about, or null when it needs no words. */
export function answerNoteKey(
  a: ContextAnswer,
):
  | 'translate.contextUnavailableAppleIntelligence'
  | 'translate.contextUnavailableNotReady'
  | 'translate.contextUnavailable'
  | 'translate.contextFailed'
  | null {
  switch (a.status) {
    case 'unavailable':
      if (a.reason === 'apple_intelligence_off')
        return 'translate.contextUnavailableAppleIntelligence';
      if (a.reason === 'model_not_ready') return 'translate.contextUnavailableNotReady';
      return 'translate.contextUnavailable';
    case 'failed':
      return 'translate.contextFailed';
    default:
      return null;
  }
}

/**
 * Applies an answer: the chat with the assistant's line (or the note that says why there is none),
 * and what the pane should show, null when nothing. `noteText` words the note for a status that
 * has one (the caller resolves answerNoteKey through its translate function).
 */
export function applyAnswer(
  c: ContextChat,
  a: ContextAnswer,
  noteText = '',
): { chat: ContextChat; shown: ShownRetranslation | null } {
  if (a.status === 'ok' && a.result.trim() !== '') {
    const text = a.result.trim();
    return {
      chat: { ...c, messages: [...c.messages, { role: 'assistant', text, engine: a.engine }] },
      shown: { text, engine: a.engine, fallback: a.fallback },
    };
  }
  const messages =
    noteText !== '' ? [...c.messages, { role: 'note' as const, text: noteText }] : c.messages;
  return { chat: { ...c, messages }, shown: null };
}

/** The retranslation to show for this text and this engine, or null: it belongs to one text, one engine. */
export function shownFor(
  b: BoundRetranslation | null,
  input: string,
  engine: string,
): BoundRetranslation | null {
  return b !== null && b.forInput === input && b.forEngine === engine ? b : null;
}

/** The default "clear context" shortcut (Mac-only, like Cmd+Enter: metaKey, never ctrlKey). */
export const DEFAULT_CLEAR_SHORTCUT = 'Cmd+Shift+K';

type Combo = { alt: boolean; ctrl: boolean; meta: boolean; shift: boolean; key: string };

/** Splits "Mod+Mod+Key" (Cmd/Command, Ctrl/Control, Shift, Alt/Option, any case) into its parts. */
function parseCombo(combo: string): Combo {
  const c: Combo = { alt: false, ctrl: false, meta: false, shift: false, key: '' };
  for (const raw of combo.split('+')) {
    const p = raw.trim().toLowerCase();
    if (p === '') continue;
    if (p === 'cmd' || p === 'command') c.meta = true;
    else if (p === 'ctrl' || p === 'control') c.ctrl = true;
    else if (p === 'shift') c.shift = true;
    else if (p === 'alt' || p === 'option') c.alt = true;
    else c.key = p;
  }
  return c;
}

/**
 * Whether a key event is the clear-context shortcut. `combo` is "Mod+Mod+Key"; modifiers must match
 * exactly. An empty combo matches nothing, and so does one without Cmd, Ctrl or Alt: a plain key
 * (or Shift+key) would fire while the user types.
 */
export function isClearShortcut(e: KeyboardEvent, combo: string = DEFAULT_CLEAR_SHORTCUT): boolean {
  const want = parseCombo(combo);
  if (want.key === '' || !(want.meta || want.ctrl || want.alt)) return false;
  return (
    e.key.toLowerCase() === want.key &&
    e.metaKey === want.meta &&
    e.ctrlKey === want.ctrl &&
    e.shiftKey === want.shift &&
    e.altKey === want.alt
  );
}

export type ShortcutCheck =
  { ok: true; value: string } | { ok: false; reason: 'needs_modifier' | 'no_key' | 'reserved' };

/** The keys the window or the system already uses for editing, translating and quitting. */
function isReserved(c: Combo): boolean {
  if (c.key === 'enter' && c.meta) return true; // Translate
  if (c.alt) return false;
  if ((c.meta || c.ctrl) && ['z', 'c', 'v', 'x', 'a', 'q', 'w'].includes(c.key)) return true;
  return c.ctrl && !c.meta && c.key === 'y'; // redo
}

/**
 * Checks the shortcut the user typed or recorded and writes it canonically (Alt, Ctrl, Cmd, Shift,
 * then the key, the order the recorder writes). Empty means "no shortcut" and is allowed.
 */
export function validateClearShortcut(raw: string): ShortcutCheck {
  if (raw.trim() === '') return { ok: true, value: '' };
  const c = parseCombo(raw);
  if (c.key === '') return { ok: false, reason: 'no_key' };
  if (!(c.meta || c.ctrl || c.alt)) return { ok: false, reason: 'needs_modifier' };
  if (isReserved(c)) return { ok: false, reason: 'reserved' };
  const key = c.key.length === 1 ? c.key.toUpperCase() : c.key[0].toUpperCase() + c.key.slice(1);
  const parts = [c.alt && 'Alt', c.ctrl && 'Ctrl', c.meta && 'Cmd', c.shift && 'Shift', key];
  return { ok: true, value: parts.filter(Boolean).join('+') };
}
