// Unicode tokenizer (issue #11 groundwork): splits arbitrary text into tokens with original
// offsets. No consumer yet: the per-word display that used it was removed when both panes became
// real textareas (issue #144). It is kept for #18 / #19, which map a textarea's selection to word
// offsets: the offsets here are UTF-16 code units, the unit of selectionStart / selectionEnd.
//
// Design notes:
// - Bottom-level segmentation is based on Intl.Segmenter (word granularity) — platform native,
//   no third-party dependency;
// - Word segments containing CJK are further split into per-character tokens by grapheme cluster
//   (the dictionary granularity of issue #18). Graphemes rather than code points are used so
//   combining marks / ZWJ emoji are never split apart;
// - Hyphens/apostrophes between two word segments merge back into one token
//   (state-of-the-art, don't);
// - Invariant: joining the tokens back must equal the original text character for character
//   (the foundation of the offset system).

export type TokenKind = 'word' | 'punct' | 'space';

export interface Token {
  /** Absolute offset in the original text (UTF-16 code units); start points at this token's first character. */
  start: number;
  end: number;
  text: string;
  kind: TokenKind;
}

/** Detects CJK (Han ideographs / kana / Hangul): word segments of this kind need further grapheme splitting. */
const CJK_RE = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u;

/** Hyphen/apostrophe: when between two words it counts as intra-word and never becomes its own token. */
const INTRA_WORD_PUNCT_RE = /^[-'’ʼ-]$/u;

let wordSegmenter: Intl.Segmenter | undefined;
let graphemeSegmenter: Intl.Segmenter | undefined;

function segmentWith(
  seg: Intl.Segmenter | undefined,
  granularity: Intl.SegmenterOptions['granularity'],
  text: string,
): Intl.SegmentData[] {
  const s = seg ?? new Intl.Segmenter(undefined, { granularity });
  const out: Intl.SegmentData[] = [];
  for (const part of s.segment(text)) out.push(part);
  return out;
}

/** Splits a single word segment containing CJK by grapheme cluster (one word token per cluster). */
function splitCjk(segment: string, start: number): Token[] {
  const graphemes = segmentWith(graphemeSegmenter, 'grapheme', segment);
  const out: Token[] = [];
  let cursor = start;
  for (const g of graphemes) {
    out.push({ start: cursor, end: cursor + g.segment.length, text: g.segment, kind: 'word' });
    cursor += g.segment.length;
  }
  return out;
}

/**
 * Splits text into a token sequence. Invariants: map(text).join('') === the original text,
 * and start/end are monotonically increasing with end-start === text.length.
 */
export function tokenize(text: string): Token[] {
  if (text === '') return [];
  wordSegmenter = wordSegmenter ?? new Intl.Segmenter(undefined, { granularity: 'word' });
  graphemeSegmenter =
    graphemeSegmenter ?? new Intl.Segmenter(undefined, { granularity: 'grapheme' });

  const raw: Token[] = [];
  for (const seg of wordSegmenter.segment(text)) {
    const start = seg.index;
    const value = seg.segment;
    if (value.trim() === '') {
      raw.push({ start, end: start + value.length, text: value, kind: 'space' });
      continue;
    }
    const wordLike = Boolean((seg as { isWordLike?: boolean }).isWordLike);
    if (wordLike && CJK_RE.test(value)) {
      raw.push(...splitCjk(value, start));
      continue;
    }
    raw.push({
      start,
      end: start + value.length,
      text: value,
      kind: wordLike ? 'word' : 'punct',
    });
  }

  // Merge pass: adjacent word tokens separated by a hyphen (Intl.Segmenter judges "state-" plus
  // its trailing hyphen as one word) or an apostrophe punct between words merge back into a
  // single token (state-of-the-art, don't).
  const merged: Token[] = [];
  for (const token of raw) {
    const prev = merged[merged.length - 1];
    if (
      prev &&
      prev.kind === 'word' &&
      (token.kind === 'word' || token.kind === 'punct') &&
      (prev.text.endsWith('-') ||
        (token.kind === 'punct' && INTRA_WORD_PUNCT_RE.test(token.text)) ||
        (!INTRA_WORD_PUNCT_RE.test(token.text) && token.text.startsWith('-')))
    ) {
      prev.end = token.end;
      prev.text += token.text;
      continue;
    }
    merged.push(token);
  }
  return merged;
}
