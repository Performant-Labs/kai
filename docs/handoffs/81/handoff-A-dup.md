# handoff-A-dup: #81 idle pane and retained session (Phase 7, anti-duplication gate)

- **Date:** 2026-09-26
- **Branch:** issue-81-implementation (worktree `.worktrees/0081-idle-and-retain`)
- **Diff base / head:** fe629dc (master) .. 270da9f (T-green)
- **Brief:** docs/handoffs/81-brief.md; Phase 3 review: handoff-A.md (PASS, 6 warn)
- **Verdict:** PASS (0 block, 3 warn)

## Summary

F extended every object the Reuse map named and built no parallel path.

- `persisted()` is reused for `kai:translate:session`. The new code has no raw `localStorage` call. The only raw calls left in `TranslateWindow.svelte` (:23-27) are the pre-existing #39 pin migration, unchanged from master.
- `resultPane.ts` was extended in place. `statusDot`/`statusDots` gained `requested`, `DotState` gained `'idle'`, and `paneState` sits beside them. The template's old inline chain conditions are gone, so "no result and not loading" is derived only in this module. The only non-test caller of `statusDots` is TranslateWindow.svelte:260.
- `translateSession.ts` is the one new object, and AC3 mandates it. It is a bindings-free leaf (a type-only import of `PaneResult`) in the `swapLangs.ts`/`flippedTarget.ts` layer. Dependency direction is unchanged: component -> utils, and utils imports no stores or bindings.
- There is one write path, the `$effect`. `clearInput` resets the fields and makes no second `store.set` (Phase 3 warn 5 applied). The store is read once through `restoreSession(get(sessionStore))`.
- `loadDefaults`/`persistLangs` are untouched.
- The closing handler's translate branch is now a no-op, and the guard and the Settings pin branch are kept (warn 6 applied).

All six Phase 3 warn defaults were applied.

## Findings

| # | Severity | File:line | Finding | Suggested fix |
|---|---|---|---|---|
| 1 | warn | frontend/src/utils/resultPane.ts:120-126 vs :527-530 | `statusDot` and `paneState` each restate the same tail rule (`results[engine]?.result` -> done/result; otherwise `requested ? failed : idle`). The two copies sit in the same module and are both pinned by tests. They differ on purpose in the loading test (entry presence vs non-empty result, Phase 3 warn 2), so the duplication is small and justified. | Leave as is. If a third consumer appears, extract a shared `noResultState(requested)` helper. |
| 2 | warn | frontend/src/components/TranslateWindow.svelte:830-846 | The `title` and `aria-label` of the dots repeat the same state -> suffix ternary. It already existed on master and grew from 3 to 4 arms in both copies. | Follow-up: derive one `dotLabel(e.value, st)` (or a `$derived` map) and bind it to both attributes. Not required for this story. |
| 3 | warn | frontend/src/components/TranslateWindow.svelte:56-58; main.go:483-487; internal/events/events.go:10-14; frontend/src/utils/events.ts:6-9 | (a) The seed cast is `as unknown as Record<string, TranslateResult>`. It is a single, commented seam, but it widens the pre-existing `PaneResult`/`TranslateResult` type hole, and `svelte-check` does not gate that hole. (b) F edited comments in three files outside the brief's Files list. The edits are comment-only and fix stale statements of the old clear-on-close behavior. No new path is added. | Accept both. O/S should note (b) in the PR body so the backend-out-of-scope line is not read as violated. |

## Notes for F

None required. The only follow-up candidate is warn 2 (dot label derivation), and it belongs to O's backlog, not this story.
