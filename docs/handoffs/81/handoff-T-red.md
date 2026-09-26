# handoff-T-red: #81 idle pane and retained session (Phase 4, RED)

- **A precondition:** handoff-A.md PASS (0 block, 6 warn). T adopted A defaults: `edited` is not a `paneState` input; `'idle'` added to `DotState`; `statusDots` also takes `requested`; single write path (`$effect`); closing handler does nothing for translate.

## Signatures pinned for F
- `paneState({ hasEngines, engine, results, loading, requested })` -> `'no-engine'|'loading'|'result'|'idle'|'failed'`. Order: no-engine > loading (loading && engine absent from results) > result (non-empty `.result`) > failed (requested) > idle.
- `statusDot(engine, results, loading, requested)`, `statusDots(engines, results, loading, requested)`; `'idle'` when not requested, not loading, no result. Existing statusDot/statusDots tests were updated to pass `requested = true` (behavior unchanged for them).
- `translateSession.ts`: `TranslateSession`, `emptySession()`, `restoreSession(raw)`.
- Component: `persisted<TranslateSession>('kai:translate:session', emptySession())`, one `$effect` writing input/results/requestedTo/requested (not loading/edited), no `localStorage.` calls, `paneState(` import from resultPane.ts, `requested = true` in doTranslate, `requested = false` in clearInput, `statusDots(..., requested)`, `translate.failed` only under a `failed` branch.

## Tests authored
| File | Tier | Pins |
|---|---|---|
| utils/resultPane.test.ts (extended) | unit | AC1, AC2: paneState (10 cases), statusDot requested rule (5) |
| utils/translateSession.test.ts (new) | unit | AC3: empty/garbage/wrong-type, JSON round-trip, requested forced false, invalid entries dropped |
| stores/translateSessionStore.test.ts (new) | integration (jsdom localStorage) | AC7: persisted read back via restoreSession, fresh-instance survival, corrupt JSON, wrong shape |
| components/sessionRetention.test.ts (new) | source contract | AC4, AC5, AC6, AC1 template use |

Guard tests that pass on master by design (pins, not RED): WindowSettings/WindowTranslate guard kept, closing handler never resets langs, EventInputFill still calls doTranslate, loadDefaults/persistLangs unchanged.

## RED confirmation
Command: `NODE_OPTIONS=--no-experimental-webstorage pnpm exec vitest run <the four files>` (from frontend/).
- resultPane.test.ts: 10 paneState cases fail `TypeError: (0 , paneState) is not a function`; statusDot idle and statusDots idle cases fail on assertions (master returns 'failed'); the rest pass.
- translateSession.test.ts, translateSessionStore.test.ts: `Failed to resolve import ".../translateSession.ts"` (accepted RED per brief).
- sessionRetention.test.ts: 10 of 15 fail on assertions, e.g. `expected 'name !== WindowTranslate) return; ...' not to match /\binput\s*=/`, `expected '{:else}' to match /failed/`, `doTranslate ... to match /\brequested\s*=\s*true/`.
Mutation reasoning: restoring the clear-on-close block fails "no longer clears"; making idle render translate.failed (bare `{:else}` or ignoring paneState) fails the failed-branch and paneState-usage tests.

Ready for F. Test files staged by explicit path, not committed.
