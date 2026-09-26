import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #116 (found in the #115 hand test): the window opened with "Translation failed" before any
// translation was made in that run. The persisted `requested` marker survives a restore whenever one
// engine's result survived, so an engine that had failed last time (its failure payload is dropped
// on restore) read as failed for a request the user does not remember. Failure is now decided from a
// run-local marker set only by a request made in this window run. Source contract, like
// resultPaneSettle.test.ts (no Svelte render harness yet, #78).
const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');

describe('a restored session never shows a failure for a request of a previous run', () => {
  it('has a run-local marker that starts false, independent of the restored session', () => {
    expect(src).toMatch(/let requestedThisRun = \$state\(false\);/);
  });
  it('doTranslate sets it and Clear resets it', () => {
    expect(src).toMatch(/requested = true;\s*requestedThisRun = true;/);
    expect(src).toMatch(/requested = false;\s*requestedThisRun = false;/);
  });
  it('the pane and the status dots read the run-local marker, not the persisted one', () => {
    expect(src).toMatch(/statusDots\(allEngines, results, awaiting, requestedThisRun\)/);
    expect(src).toMatch(/paneState\(\{[\s\S]*?requested: requestedThisRun,/);
  });
});
