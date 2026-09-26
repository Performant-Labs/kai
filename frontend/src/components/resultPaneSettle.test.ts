import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Wiring contract for the two result-pane fixes found while hand-testing #82 (the pure rules are in
// utils/resultPaneSettle.test.ts). No Svelte render harness exists yet (#78), so this reads the
// component source, like swapWindow.test.ts and sessionRetention.test.ts.
const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');

describe('a fast engine answering first does not make the active engine read as failed', () => {
  it('the result handler ends the wait only when every enabled engine has reported', () => {
    expect(src).toMatch(/if \(allReported\(allEngines, results\)\) awaiting = false;/);
  });
  it('doTranslate opens the wait, Clear closes it, and the 15 s fallback closes it too', () => {
    expect(src).toMatch(/loading = true;\s*awaiting = true;/);
    // #116 puts `requestedThisRun = false;` between the two; the wait is still closed by Clear.
    expect(src).toMatch(/requested = false;\s*requestedThisRun = false;\s*awaiting = false;/);
    expect(src).toMatch(/anyPending\(allEngines, results, awaiting\)\) awaiting = false/);
  });
  it('the pane and the status dots read the wait, not the first-result loading flag', () => {
    expect(src).toMatch(/paneState\(\{[\s\S]*?loading: awaiting,/);
    expect(src).toMatch(/statusDots\(allEngines, results, awaiting, requestedThisRun\)/);
  });
});

describe('the engine dropdown does not show every engine as disabled', () => {
  it('options are disabled from the GetAllEngines list, not from the missing enabled field', () => {
    expect(src).toMatch(/disabled=\{isEngineOptionDisabled\(e\.value, allEngines\)\}/);
    expect(src).not.toMatch(/disabled=\{!e\.enabled\}/);
    expect(src).toContain('engineOptionLabel(');
  });
});

// Hand test of #82 (2026-09-26): picking another engine in the result-pane dropdown did not switch
// the pane. activeEngine was derived from activeEngineFor(LAST_ENGINE_KEY, ...), which reads
// localStorage directly, so the derivation had no reactive dependency on the pick; the dropdown
// showed the new engine until the next render (Translate) snapped it back to the old one.
describe('picking an engine in the result-pane dropdown switches the pane', () => {
  it('the active-engine derivation depends on the last-used store', () => {
    const m = src.match(/const activeEngine = \$derived[\s\S]*?;\n/);
    expect(m, 'activeEngine derivation not found').not.toBeNull();
    expect(m![0]).toContain('$lastUsedStore');
  });
});
