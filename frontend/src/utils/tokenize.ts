// Unicode 分词器（issue #11 基础工作）：把任意文本切成带原始偏移的 token，
// 供两栏的 hover 高亮 span 渲染使用（点击行为属于后续 issue #18，这里刻意不做）。
//
// 设计要点：
// - 基于 Intl.Segmenter（word 粒度）做底层切分——平台原生、无第三方依赖；
// - 含 CJK 的 word 段再按 grapheme 簇拆成单字 token（issue #18 的字典粒度）；
//   用 grapheme 而非码点，保证组合符号 / ZWJ emoji 不被拆散；
// - 连字符/撇号夹在两个 word 段之间时合并回一个 token（state-of-the-art、don't）；
// - 不变量：token 拼回必须逐字符等于原文（spanText 渲染与偏移系统的根基）。

export type TokenKind = 'word' | 'punct' | 'space';

export interface Token {
  /** 原文中的绝对偏移（UTF-16 码元），start 指向本 token 首字符。 */
  start: number;
  end: number;
  text: string;
  kind: TokenKind;
}

/** 含 CJK（中日韩文字/假名/谚文）的判定：这类 word 段需要按 grapheme 再细分。 */
const CJK_RE = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u;

/** 连字符/撇号：夹在两个 word 之间时视作词内部，不独立成 token。 */
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

/** 单个 word 段含 CJK 时按 grapheme 簇拆分（每簇一个 word token）。 */
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
 * 把文本切分为 token 序列。不变量：map(text).join('') === 原文，
 * 且 start/end 单调递增、end-start === text.length。
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

  // 合并遍：相邻 word token 以连字符为界（Intl.Segmenter 会把 "state-" 连尾随连字符一起
  // 判成 word）或撇号 punct 夹在 word 之间时，合并回一个 token（state-of-the-art、don't）。
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
