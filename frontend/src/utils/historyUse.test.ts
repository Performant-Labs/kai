import { describe, expect, it } from 'vitest';
import { toUsePayload, useMapping, type UseContext } from './historyUse.ts';
import type { HistoryRow } from './historyList.ts';

// Issue #60: "Use" puts an entry back into the translate window: the source, the result, the two
// languages and the engine. It never starts a translation, which is the window's business.
const entry: HistoryRow = {
  id: 7,
  text: 'Necesito los puntos clave',
  result: 'I need the key points',
  from_lang: 'es-MX',
  to_lang: 'en',
  engine: 'google',
  engine_id: 2,
  from_ocr: true,
  created_at: 1,
};
const ctx: UseContext = {
  languages: ['auto', 'en', 'es-MX', 'fr'],
  engines: ['system', 'google'],
  active: 'system',
};

describe('toUsePayload', () => {
  it('carries what is needed to restore the entry, and nothing about the row', () => {
    expect(toUsePayload(entry)).toEqual({
      text: 'Necesito los puntos clave',
      result: 'I need the key points',
      from_lang: 'es-MX',
      to_lang: 'en',
      engine: 'google',
    });
  });
});

describe('useMapping', () => {
  it('restores the source, the languages and the engine', () => {
    const m = useMapping(toUsePayload(entry), ctx);
    expect(m.input).toBe('Necesito los puntos clave');
    expect(m.from).toBe('es-MX');
    expect(m.to).toBe('en');
    expect(m.engine).toBe('google');
    expect(m.requestedTo).toBe('en');
  });
  it('puts the result under its engine, as the engine would have reported it', () => {
    const m = useMapping(toUsePayload(entry), ctx);
    expect(Object.keys(m.results)).toEqual(['google']);
    expect(m.results.google).toMatchObject({
      engine: 'google',
      result: 'I need the key points',
      from: 'es-MX',
      to: 'en',
    });
  });
  it('shows the result under the engine now selected when the entry engine is gone', () => {
    const m = useMapping({ ...toUsePayload(entry), engine: 'deepl' }, ctx);
    expect(m.engine).toBe('system');
    expect(Object.keys(m.results)).toEqual(['system']);
  });
  it('does the same when the entry has no engine', () => {
    const m = useMapping({ ...toUsePayload(entry), engine: '' }, ctx);
    expect(m.engine).toBe('system');
  });
  it('leaves a language alone that the window cannot show', () => {
    const m = useMapping({ ...toUsePayload(entry), from_lang: 'xx', to_lang: '' }, ctx);
    expect(m.from).toBeNull();
    expect(m.to).toBeNull();
  });
  it('never sets the target to auto', () => {
    const m = useMapping({ ...toUsePayload(entry), to_lang: 'auto' }, ctx);
    expect(m.to).toBeNull();
  });
  it('accepts auto as a source', () => {
    const m = useMapping({ ...toUsePayload(entry), from_lang: 'auto' }, ctx);
    expect(m.from).toBe('auto');
  });
  it('restores no result when the entry has none, so the pane stays idle', () => {
    const m = useMapping({ ...toUsePayload(entry), result: '' }, ctx);
    expect(m.results).toEqual({});
  });
  it('restores no result when there is no engine to show it under', () => {
    const m = useMapping(toUsePayload(entry), { ...ctx, engines: [], active: '' });
    expect(m.results).toEqual({});
    expect(m.engine).toBe('');
  });
});
