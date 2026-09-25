import { describe, expect, it } from 'vitest';
import { tokenize, type Token } from './tokenize.ts';

// Reassembly invariant: tokens cut from any text must join back to exactly the original text
// (the foundation of spans rendering).
function expectReassembles(text: string) {
  const tokens: Token[] = tokenize(text);
  expect(tokens.map((t) => t.text).join('')).toBe(text);
  let cursor = 0;
  for (const t of tokens) {
    expect(t.start).toBe(cursor);
    expect(t.end).toBe(t.start + t.text.length);
    cursor = t.end;
  }
  return tokens;
}

describe('tokenize — offset integrity', () => {
  it('reassembles plain English exactly', () => {
    expectReassembles('Hello, world!');
  });

  it('reassembles CJK, emoji and combining marks exactly', () => {
    expectReassembles('你好世界 👨‍👩‍👧 cafe\u0301 test');
  });
});

describe('tokenize — word integrity', () => {
  it('keeps internal apostrophes inside one word token', () => {
    const tokens = tokenize("don't panic");
    const words = tokens.filter((t) => t.kind === 'word').map((t) => t.text);
    expect(words).toContain("don't");
  });

  it('keeps hyphenated compounds inside one word token', () => {
    const tokens = tokenize('a state-of-the-art design');
    const words = tokens.filter((t) => t.kind === 'word').map((t) => t.text);
    expect(words).toContain('state-of-the-art');
  });

  it('keeps combining marks attached to their base', () => {
    const tokens = tokenize('cafe\u0301');
    const words = tokens.filter((t) => t.kind === 'word').map((t) => t.text);
    expect(words).toContain('cafe\u0301');
  });
});

describe('tokenize — CJK', () => {
  it('segments CJK per grapheme cluster', () => {
    const tokens = tokenize('你好世界');
    const words = tokens.filter((t) => t.kind === 'word').map((t) => t.text);
    expect(words).toEqual(['你', '好', '世', '界']);
  });

  it('handles mixed CJK + latin with word tokens on both sides', () => {
    const tokens = tokenize('Hello 世界!');
    const words = tokens.filter((t) => t.kind === 'word').map((t) => t.text);
    expect(words).toEqual(['Hello', '世', '界']);
  });
});

describe('tokenize — kinds', () => {
  it('classifies whitespace, words and punctuation', () => {
    const tokens = tokenize('Hi. ok');
    expect(tokens.map((t) => t.kind)).toEqual(['word', 'punct', 'space', 'word']);
  });

  it('treats an astral emoji cluster as one single token', () => {
    const tokens = tokenize('👨‍👩‍👧 ok');
    expect(tokens[0].text).toBe('👨‍👩‍👧');
    expect(tokens[0].kind).toBe('punct');
  });
});
