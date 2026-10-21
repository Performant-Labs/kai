# Handoff-S: Phase 9 — Spec audit, issues #112 + #113 (run 0112-issue-112)

**Date:** 2026-10-21
**Branch / worktree:** issue-0112-issue-112 (`.worktrees/0112-issue-112`)
**Diff audited:** `git diff master..HEAD` (HEAD `d21c8de`, incl. every commit through the A-dup docs commit; code head `5d8b149` + T's B-1 test commit `f5f75e0`)
**Spec:** issues #112 + #113 (read in full via `gh issue view`) as interpreted by `brief.md` (AC1–AC10, amended) + handoff-A round-1/round-2 rulings (R2-1 binding)
**Handoff-T reviewed:** `handoff-T-green.md` (T-red cross-read for the contract inventory)
**Handoff-A reviewed:** `handoff-A.md` (round 1 BLOCK → round 2 PASS), `handoff-A-dup.md` (PASS)
**Handoff-F reviewed:** `handoff-F.md` (§"Disclosed for S" ruled below)
**Operator-facing report:** N/A — `uiSurface: false` (declared at pre-flight; brief.md:3-5); non-visual audit, markdown handoff only.

## Preconditions

| Gate | Status |
|---|---|
| A up-front | **PASS** (round 1 BLOCK on AC7×AC8, remediated in the amended brief; round 2 PASS, 0 block, R2-1 recorded binding) |
| A anti-duplication | **PASS** (0 block / 1 nit; every reuse-map row honored, `persisted.ts` + `pinKey()` byte-identical, aux windows untouched) |
| T structural | **PASS** — full `test.unit.command` GREEN in a foreground, run-unique log (`SUITE_EXIT=0`, 20 Go `ok`, 82 files / 889 tests, both release harnesses `PASS`); zero blocking issues |
| Browser / visual-diff | **N/A, declared** — no visual surface (brief.md:3); no Tier 3 pixel work applies |

## Targeted re-runs (this audit session, this worktree)

| Command | Result |
|---|---|
| `go test -vet=off ./internal/service/... -count=1` | `ok cnb.cool/dtapp/kai/internal/service 2.326s` |
| `go test -vet=off . -count=1` (root pkg — the 7 wiring/param tests) | `ok cnb.cool/dtapp/kai 0.993s` |
| `pnpm --dir frontend test` | `Test Files 82 passed (82)` · `Tests 890 passed (890)` |

890 = T-green's 889 + exactly 1: the diff-gate B-1 remediation test (`f5f75e0`, mounted AC7(b), authored by **T** per decisions.md:256). Growth fully accounted; the full 11-minute suite was not re-run (GREEN at t-green by both T and the plugin — accepted, not re-verified).

## Acceptance criteria — AC by AC

| AC | Requirement | Verdict | Evidence |
|---|---|---|---|
| **#112 AC1** | Close button **hidden** (`ButtonHidden`), translate only; Settings/History/Screenshot unchanged; per-platform rendering manual | **PASS** (wiring) / per-platform rendering **NOT-VERIFIABLE-HERE** (manual, W-3) | `main.go:672` `CloseButtonState: application.ButtonHidden` (was `ButtonEnabled`); own grep: `:546/:607/:750` still `ButtonEnabled`. Test `TestTranslateWindowCloseButtonIsHidden` (`main_windowsize_test.go`) asserts all four option blocks, translate-only. |
| **#112 AC2** | Named unit-tested rule in `window_wrapper.go` = broadcast `EventWindowClosing(WindowTranslate)` + Hide; hook composes `event.Cancel()` + rule; tray `Hide` stays broadcast-free; no new file | **PASS** | `HideTranslateWindow(win hideWindow, emit)` at `window_wrapper.go:176-189` (nil no-op, one broadcast, one Hide), placed after `LowerForSettings` in the file's own cadence; hook `main.go:704-714` = `event.Cancel()` + the rule, inline Emit deleted; tray toggle untouched. Tests: `TestHideTranslateWindowBroadcastsWindowClosingForTheTranslateWindow` / `…HidesWithoutDestroying` / `…WithoutAWindowIsANoOp` (`window_close_test.go`) + `TestTranslateWindowClosingHookComposesCancelThenTheCloseRule` (asserts the forbidden inline-Emit literal is absent). |
| **#112 AC3** | Cmd+W deterministically hides via the **same** rule; binding registered via `RegisterKeyBinding("Cmd+W", …)` in `main.go`; registration/firing manual (W-1) | **PASS** (wiring) / firing **NOT-VERIFIABLE-HERE** (manual) | `main.go:715-719`: comma-ok narrowing to `*application.WebviewWindow` (compile-forced, disclosed, always succeeds), callback = `service.HideTranslateWindow(window, emitForCloseRule)`. Test `TestTranslateWindowCmdWBindingCallsTheCloseRule` (asserts the callback calls the rule, not a bare Hide). |
| **#112 AC4** | Bring-back paths stay green (tray toggle, input hotkey show, hook's own hide) | **PASS** | `window_toggle_test.go` (tray toggle) green in the `internal/service` re-run; hotkey-show/`translateShortcut`, `historyUseRestore`, `sessionRetention` suites green in the 82/890 re-run; the hook's hide is the AC2 rule itself. |
| **#112 Cmd+Q** | Quitting untouched | **PASS** | No quit/Cmd+Q path appears anywhere in the diff (main.go hunks touch only the translate options block, the WindowClosing hook, the new binding). |
| **#113 AC5** | `translateWindowStartsPinned(settingsService) bool` in `main_windowsize.go` (no new file; comment widened); off→false, on→true, nil/unreadable→false; fed into `AlwaysOnTop:` | **PASS** | `main_windowsize.go:53-68` (nil svc→false, nil cfg→false, else `cfg.TranslateWindowOnTop`); file comment widened to "creation params … (size, level)" (A Finding 5 honored); `main.go:655` `AlwaysOnTop: translateWindowStartsPinned(settingsService)`. Tests: `TestTranslateWindowStartsPinned_OffByDefaultStartsAtTheNormalLevel` / `…_OnStartsAlwaysOnTop` / `…_NilServiceStartsAtTheNormalLevel` + `TestTranslateWindowCreationFeedsStartsPinnedIntoAlwaysOnTop`. (The nil-*config* branch is a two-line defensive guard mirroring `translateWindowSize`'s equally untested one — pre-existing shape, not a finding.) |
| **#113 AC6** | Pin under translate-specific versioned key `kai:translate:pinned:v2`; written **only** on explicit 🧷; mount-init resolution never reaches localStorage (A Finding 4); `initialPin` semantics stay; `pinKey()` untouched; one-time reset CHANGELOG'd | **PASS** | `stores/translatePin.ts` (new, the sanctioned "pin-specific sibling"): v2-only read (corrupt→false, never rewritten), `subscribe`/`apply` never write, `set`/`update` the only persist path. `TranslateWindow.svelte:30-36` swaps the store; `togglePin` keeps the single `.set` (own grep: one `pinnedStore.set` at `:36`, one `.apply` at `:54`). `persisted.ts` diff-empty, `pinKey()` byte-identical (own grep). Tests: `translatePin.test.ts` 9/9 incl. "a mount with nothing stored leaves localStorage EMPTY" (Finding 4) and old-key-ignored; mounted: "an old-key pin is pollution, not a choice", "a mount with nothing stored leaves localStorage empty", "an explicit 🧷 toggle is the only write". CHANGELOG line 2 carries the one-time reset. |
| **#113 AC7** | Toggling the setting broadcasts (EventEnginesChanged shape); **effective-pin rule** (R2-1: `pinTouched ? thisMountChoice : initialPin(storedV2, liveSetting)`) at **every** application point — mount init, broadcast, re-pin; matrix cases (a) and (b) | **PASS** | `effectivePin` in `pinDefault.ts:36-38` — R2-1 verbatim, composes `initialPin` (does not replace it). One resolution path `resolvePinLevel()` (`TranslateWindow.svelte:44-63`, re-fetches GetConfig, payload not trusted) feeding all three points: mount `await reapplyPin()` (`:958`), broadcast listener `:947`, re-pin `:898`. `GeneralTab.svelte:136-137` emits after `SaveConfig` inside the try (failed save → no broadcast); off-fn in onMount cleanup (`:993`). Tests: `effectivePin.test.ts` 4/4 (the R2-1 decision table incl. the unspecified cell — stored v2 untouched beats the setting, both directions); mounted `#113 AC7(a)` (follows the new setting through the re-pin) and `#113 AC7(b)` (explicit choice survives setting-toggle OFF→ON **and** the Settings-close re-pin — the discriminating direction; diff-gate B-1's demanded test); `generalTabOnTop.test.ts` "toggling the switch broadcasts kai:translate-window-on-top:changed"; `events.test.ts` (the name constant). |
| **#113 AC8** | Settings/History-close re-pin keeps working, rewritten to resolve through the effective-pin rule; test exercises the re-pin with the **versioned** key present (W-2) | **PASS** | `TranslateWindow.svelte:895-901`: `void resolvePinLevel().then(() => Window.SetAlwaysOnTop($pinnedStore)…)` — decision first, then apply (the literal `SetAlwaysOnTop($pinnedStore)` line is retained because the pre-existing `translateWindowGear.test.ts` pins it; the applied value is the just-resolved decision — A-dup re-verified the composition). Tests with v2 present: `translatePin.test.ts` "AC8: with a versioned choice present, the Settings-close re-pin resolves through effectivePin and does not revert it" (no write); mounted `historyUseRestore.test.ts:320` re-pin (seeded v2 after T's sanctioned repair — not only the old-key migration case, W-2 satisfied); mounted AC7(a) covers the settings-close leg, historyUseRestore the history-close leg. |
| **AC9** | CHANGELOG `[Unreleased]`: two lines (not-closable window; on-top follows setting + one-time pin reset), `(#112)`/`(#113)` | **PASS** | Verified verbatim in the diff: two lines added under `## [Unreleased]`, `(#112)` and `(#113)` cited, existing line style, content matches the user-visible changes exactly (incl. "The stored pin choice moved to a new key and starts fresh — a one-time reset"). |
| **AC10** | Full suite green at t-green; no `-skip` added; no test edits by F | **PASS** | T-green foreground log: `SUITE_EXIT=0`, 82 files/889 tests, plugin GREEN concurred. `-skip` grep over the diff's test files: zero hits. F zero-test-edit proof (T-green per-file table) holds — the one post-T-green test commit `f5f75e0` (B-1) is T's, recorded at decisions.md:256. |

**Interpretation fidelity notes:** the implementation follows the amended brief exactly, including all five binding A rulings — Finding 1 (effective-pin rule at every point, re-pin never applies the raw store), Finding 2 (broadcast+Hide rule, both entry points delegate), Finding 3 (`kai:translate:pinned:v2`, `pinKey()` untouched), Finding 4 (mount-path no-write test), Finding 5 (AC5 stays in `main_windowsize.go`) — and R2-1's formula verbatim (`effectivePin` composes `initialPin`; the stored-v2-untouched cell keeps winning through the re-pin — pinned at pure, store, and mounted tiers).

## Not verifiable here (for the PR description's manual list)

1. **Per-platform close-button rendering** — macOS traffic-light close actually not shown; Windows title-bar close item actually not shown (`ButtonHidden` is wired and source-asserted; rendering needs the signed app). [W-3]
2. **Cmd+W actually firing** on the real app — the binding's registration and callback wiring is tested; key-binding dispatch needs the signed app. [W-1] PR wording nuance (F's disclosure): Wails parses `"Cmd+W"` as CmdOrCtrl, i.e. **Ctrl+W on Windows** — hide-only either way.
3. **The window actually going behind other apps** with the setting off (the real level drop #113 asks to "verify against the real app").
4. **Tray / hotkey bring-back on a real app** after a Cmd+W or hook hide (paths are unit/mounted-tested; the real signed-app round trip is manual).

## Scope check

**PASS — nothing outside the guard.** All 32 diff files:

- **Production (guard-named):** `main.go`, `internal/service/window_wrapper.go`, `main_windowsize.go`, `frontend/src/components/TranslateWindow.svelte`, `frontend/src/components/settings/GeneralTab.svelte`, `frontend/src/stores/translatePin.ts` (the brief's "or a pin-specific sibling"), `frontend/src/utils/pinDefault.ts`, `frontend/src/utils/events.ts`, `CHANGELOG.md` (AC9).
- **Tests:** `window_close_test.go` + `main_windowsize_test.go` (guard's "+ its tests"/"(+test)"); new companions of guard-named sources (`translatePin.test.ts`, `effectivePin.test.ts`, `events.test.ts`); mounted companions of guard-named components (`translateWindowPinDefault.test.ts`, `generalTabOnTop.test.ts`); and `historyUseRestore.test.ts` — the one file not derivable from the guard's text. **Ruled in-scope:** it tests exactly AC8's re-pin behavior on the in-scope component; its single-line change (`:320`, old key → v2) was *mandated* by the run's own binding AC6 re-contract (seeding the retired key and expecting `true` contradicts R2-1 + v2-only reads); it is T-owned (sanctioned repair at T-green, git-diff verified, sibling assertions untouched) and was subsequently reviewed by the diff gate and A-dup. Recorded here as the run's narrowest judgment call, not a violation.
- **Run records (allowed per O):** `docs/handoffs/0112-issue-112/**` (17 files: brief, survey, decisions, A/A-dup/F/T handoffs, dual-review artifacts).

## F's disclosed nuances + config disclosures — rulings

| Disclosure | Ruling |
|---|---|
| Cmd+W → Ctrl+W on Windows (`CmdOrCtrlKey` parsing) | **Acknowledged** — PR-description wording only; the literal `"Cmd+W"` is the contract (A verified the parsing); hide-only, harmless. |
| Manual items stay manual (W-1/W-3) | **Acknowledged** — matches the brief's own guard; enumerated in the not-verifiable-here list above. |
| Re-pin re-fetches `GetConfig` on every Settings/History close | **Acknowledged** — required by R2-1's "evaluated live at every application point"; one in-memory settings read per close event; negligible. |
| `get(pinnedStore)` as `thisMountChoice` (invariant: `togglePin` is the only `.set` caller) | **Acknowledged** — re-verified by my own grep (`:36` sole `pinnedStore.set`, `:54` sole `.apply`). Unpinned at the Svelte tier; T-green advisory 4's cheap future guard noted below. |
| `emitForCloseRule` adapter (compile-forced: `Event.Emit` returns `bool`, the rule's tested shape returns none) | **Acknowledged** — T-green accepted with the right precedence (tests over BINDING prose); defined once, passed at both sites; assertion-invisible (verified in the diffs). |
| `*WebviewWindow` comma-ok narrowing for `RegisterKeyBinding` (compile-forced) | **Acknowledged** — always succeeds (`NewWithOptions` returns the concrete type); preserves the pinned `NewWithOptions` anchor test; assertion-invisible. |
| `svelte-check` cannot run in this environment | **Acknowledged — reproduced by S** (crashes inside its own bin under Node 26/TS 7, before reading any project file → cannot be diff-caused; pre-existing; not part of `test.unit.command`). Vitest compiled every touched component (82/890). |
| T/F bash-edited assigned files (generated-allowlist gaps: `paths.production` missing root `main_*.go` + `CHANGELOG.md`; `paths.test` missing frontend test globs) | **Acknowledged** as process disclosure — every such path is inside the brief's scope guard; edits were git-diff-verified when made; owner follow-up already recorded (third occurrence). Not a spec violation of this story. |
| Diff-gate NV-5 corner (a stored v2 choice contradicting the setting keeps winning until the next explicit click) | **Acknowledged** — that is #69's locked rule applied to v2, exactly AC6's "`initialPin` semantics stay"; gate-verified and bounded; worth one PR-description sentence. |

## Quality audit

| Area | Result | Notes |
|---|---|---|
| Architecture gate | PASS | A round-2 PASS + A-dup PASS; no quality finding here conflicts with either. |
| Code organization | PASS | Every addition extends an existing cadence (package-level rule, creation-param sibling, event constant, store sibling); comments in each file's own idiom. One stale-comment nit (advisory 1). |
| Test quality (rubric §7) | PASS | Tiered and proportionate: pure decision table (4) → store contract (9) → mounted sequences (AC6/AC7(a)/(b), re-pin) → wiring source-assertions (4) + creation-param (3). Each test names one behavior and fails in isolation for the named reason; assertions are behavioral (setAlwaysOnTop calls, localStorage bytes, emit counts/hides) except the deliberately source-level main.go wiring pins — the repo's established idiom for un-live-Wails-testable wiring, with the tradeoff documented in the tests' own comments. The mounted AC7(b) test discriminates in the right direction (clicked value vs contradicting live setting). `events.test.ts`'s constant pin is the narrowest defensible case (the name **is** the cross-file contract, and the RED mechanics required the literal — documented). No assertion-free, duplicate-signal, or coverage-padding tests; **no delete/merge findings**. |
| Error handling | PASS | Store read/write try/catch'd with corrupt→false + no repair-write; `resolvePinLevel` keeps the current level on config failure ("never override a click"); failed `SaveConfig` broadcasts nothing. |
| Security | N/A | No new inputs, no auth surface, no secrets; localStorage is the only persistence touched. |
| Performance | PASS | One extra in-memory config read per Settings/History close; no hot-path additions. |
| Naming consistency | PASS | `HideTranslateWindow`, `translateWindowStartsPinned`, `effectivePin`, `EventTranslateWindowOnTopChanged` (`kai:<thing>:changed` cadence) all follow the repo's terms and the issue's vocabulary. |
| API consistency | N/A | No HTTP/API surface in this story. |
| UI/UX match to spec | PASS (as declarable) | No new layout (uiSurface false); the only UI-adjacent deltas are the removed close affordance and the reused settings row — both as specified. |
| Accessibility | N/A | No new controls; the 🧷 button keeps its existing aria-labels (tests query by them). |
| Visual regression | N/A | No visual surface (declared). |
| Scope | PASS | See Scope check above. |

## Verdict

**PASS** — all ten ACs verified against the amended brief and the binding A rulings (R2-1 included), with the four real-app items explicitly carried as manual verification; CHANGELOG accurate; scope clean; F's and T's disclosures all acknowledged; targeted suites re-run green (internal/service `ok`, root `ok`, frontend 82/890). Ready for O to commit and open the PR (milestone 0.6.1, closing #112 and #113).

## Advisory notes (non-blocking)

1. **Stale doc comment** `window_wrapper.go:176-178` — "main.go passes `app.Event.Emit` itself as the value" is false since the `emitForCloseRule` adapter (= T-green advisory 3 / A-dup nit 1). One-line fix on the file's next touch; no assertion reads it.
2. **Pipeline-config owner follow-up (third occurrence):** `paths.production` lacks root `main_*.go` + `CHANGELOG.md`; `paths.test` lacks frontend test globs; `test.unit.command`'s Go pattern omits the root package (7 of this story's tests — T-green closed it manually both times). Deserves its own maintenance item.
3. **Cheap future guard** for the `.set`-invariant: a source-assertion test counting `pinnedStore.set(` call sites in `TranslateWindow.svelte` (T-green advisory 4).
4. **PR description reminders:** the manual list above; the Ctrl+W-on-Windows wording; the NV-5 corner sentence; #113's own "verify against the real app" ask maps to manual item 3.
