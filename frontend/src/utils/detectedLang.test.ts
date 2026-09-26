import { describe, expect, it } from 'vitest';
import { detectedSourceLabel } from './detectedLang.ts';

const AUTO = 'auto';

const nameOf = (code: string) =>
  ({ en: 'English', zh: 'Chinese', es: 'Spanish', 'es-MX': 'Spanish (Mexico)' })[code] ?? code;
const SUFFIX = ' (detected)';

describe('detectedSourceLabel', () => {
  it('returns null when the source language is pinned (not auto)', () => {
    expect(detectedSourceLabel('en', AUTO, 'es', nameOf, SUFFIX)).toBeNull();
  });

  it('returns null when nothing has been detected yet', () => {
    expect(detectedSourceLabel('auto', AUTO, '', nameOf, SUFFIX)).toBeNull();
  });

  it('labels the detected language with the suffix while auto', () => {
    expect(detectedSourceLabel('auto', AUTO, 'es', nameOf, SUFFIX)).toBe('Spanish (detected)');
  });

  it('falls back to the raw code when the detected language is unknown', () => {
    expect(detectedSourceLabel('auto', AUTO, 'xx', nameOf, SUFFIX)).toBe('xx (detected)');
  });

  it('ignores an empty-suffix edge without inserting a stray space', () => {
    expect(detectedSourceLabel('auto', AUTO, 'es', nameOf, '')).toBe('Spanish');
  });

  it('returns null when the service fell back to auto (nothing detected)', () => {
    expect(detectedSourceLabel('auto', AUTO, 'auto', nameOf, SUFFIX)).toBeNull();
  });

  it('keeps the (detected) suffix on a qualified variant label', () => {
    expect(detectedSourceLabel('auto', AUTO, 'es-MX', nameOf, SUFFIX)).toBe(
      'Spanish (Mexico) (detected)',
    );
  });
});
