// The frontend half of the source-text correction (issue #208).
//
// When the "Correct grammar and wording" checkbox is on, the backend (translate.Service.CorrectSource)
// decides everything: the setting, the language, the length floor, the output guards and the word
// diff. This window compares no languages and counts no characters. What lives here is the small
// pure gate between the backend's answer and the window, so vitest covers it without the bindings
// (like sourceSwitch.ts): whether an answer can be applied, whether it still describes the window,
// and which text a translation is sent with.
//
// Nothing here persists or teaches anything: the corrected text is never a default and never
// reaches the language variant store.

/** One place the correction changed the text (model.TextChange). */
export interface TextChange {
  before: string;
  after: string;
}

/** The backend's answer (model.Correction); only the fields this window reads. */
export interface CorrectionAnswer {
  corrected: boolean;
  status?: string;
  reason?: string;
  text: string;
  original: string;
  language?: string;
  changes: TextChange[] | null;
}

/** A correction the window applied: the text as it arrived, what it became, and what differs. */
export interface AppliedCorrection {
  original: string;
  text: string;
  language: string;
  changes: TextChange[];
}

/**
 * The correction to apply for `sent` (the text the request carried), or null when the answer must
 * not be applied: nothing corrected, blank or unchanged text, no changes to show, or an answer for
 * another text than the one that was sent.
 */
export function acceptCorrection(
  answer: CorrectionAnswer | null | undefined,
  sent: string,
): AppliedCorrection | null {
  if (!answer || !answer.corrected) return null;
  if (answer.original !== sent) return null;
  if (answer.text.trim() === '' || answer.text.trim() === sent.trim()) return null;
  if (!answer.changes || answer.changes.length === 0) return null;
  return {
    original: answer.original,
    text: answer.text,
    language: answer.language ?? '',
    changes: answer.changes.map((c) => ({ before: c.before, after: c.after })),
  };
}

/** Whether the correction still describes the window: the source text is the one it was made for. */
export function correctionShown(
  c: AppliedCorrection | null,
  input: string,
): c is AppliedCorrection {
  return c !== null && c.original === input;
}

/** The text a translation is sent with: the corrected text while the correction applies, else what is on screen. */
export function textToSend(c: AppliedCorrection | null, input: string): string {
  return correctionShown(c, input) ? c.text : input;
}

/** The i18n key that words a reason the correction is unavailable (engine.CorrectionStatus). */
export function unavailableKey(
  reason: string,
):
  | 'translate.correctUnavailableAppleIntelligence'
  | 'translate.correctUnavailableNotReady'
  | 'translate.correctUnavailableHardware'
  | 'translate.correctUnavailablePlatform'
  | 'translate.correctUnavailable' {
  switch (reason) {
    case 'apple_intelligence_off':
      return 'translate.correctUnavailableAppleIntelligence';
    case 'model_not_ready':
      return 'translate.correctUnavailableNotReady';
    case 'unsupported_hardware':
      return 'translate.correctUnavailableHardware';
    case 'unsupported_platform':
      return 'translate.correctUnavailablePlatform';
    default:
      return 'translate.correctUnavailable';
  }
}
