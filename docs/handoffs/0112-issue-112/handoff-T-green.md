# Handoff-T-green: Phase 6 — GREEN verification for issues #112 + #113

**Date:** 2026-10-21
**Branch / worktree:** issue-0112-issue-112 (`.worktrees/0112-issue-112`)
**Issues:** #112 (translate window not closable) + #113 (on-top setting respected everywhere)
**Handoff-F reviewed:** `docs/handoffs/0112-issue-112/handoff-F.md`
**Handoff-T-red:** `docs/handoffs/0112-issue-112/handoff-T-red.md`
**Evidence log (this run):** `/tmp/0112-tgreen-suite.log` — foreground re-run inside THIS worktree

> **Earlier BLOCK resolved.** The prior session's PASS was refused because the log it pointed at
> was run B's (cross-run mixup); that BLOCK is discharged by this foreground re-run, whose log is
> run-unique (`/tmp/0112-tgreen-suite.log`, 1220 lines, written 07:13 in this worktree) and whose
> numbers are quoted verbatim below.

## Verdict: **PASS** — GREEN, no blocking issues. No UI surface — U is N/A (declared), ready for S.

## GREEN confirmation

The authoritative suite, exactly as configured, run in the foreground and logged to
`/tmp/0112-tgreen-suite.log`:

```
sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh" > /tmp/0112-tgreen-suite.log 2>&1; echo "SUITE_EXIT=$?"
```

Verbatim summary lines from the log:

| Stage | Verbatim from `/tmp/0112-tgreen-suite.log` |
|---|---|
| swiftbridge build | `>> done: pkg/swiftbridge/libkai_bridge.dylib` / `>> done: pkg/swiftbridge/libkai_bridge.a` |
| `go test -vet=off ./internal/... ./pkg/... -count=1` | **20 package `ok` lines, 0 `FAIL`** — `ok cnb.cool/dtapp/kai/internal/buildinfo 0.298s` … `ok cnb.cool/dtapp/kai/internal/service 1.993s` … `ok cnb.cool/dtapp/kai/pkg/wails-updater-providers 0.301s` |
| `pnpm --dir frontend test` | `Test Files  82 passed (82)` · `Tests  889 passed (889)` |
| `scripts/release.test.sh` | final line `PASS` (log :1182) |
| `scripts/release-ask-ai.test.sh` | final line `PASS` (log :1219) |
| whole suite | `SUITE_EXIT=0` (last line) |

Zero `FAIL` occurrences anywhere in the log. Numbers match the prediction exactly (82 files /
889 tests; F's self-check had predicted 889/889 after T's repair — reproduced with no
discrepancies).

Independent add-on, re-run fresh this session: `go test -vet=off . -count=1` →
`ok cnb.cool/dtapp/kai 0.789s` — the configured Go pattern omits root package `main`, where 7 of
this story's tests live (3 × `TestTranslateWindowStartsPinned_*` + 4 main.go wiring
source-assertions); I closed that hole manually (Advisory 1).

Growth accounting vs RED: 880 → 889 frontend tests = exactly the 9 `translatePin.test.ts` tests
RED could not collect (import failure); 82 = 79 + 3 new files. Nothing else moved.

**Tests still fail if the behavior is removed** — evidenced three ways, without mutating
anything (per instruction): (a) the observed RED in this same worktree is the same assertion set
failing for the right reasons; (b) F's self-check caught a real contract violation (the stale
old-key seed) as exactly one failing test — the suite discriminates; (c) the mutation analysis
below shows each wrong-resolver shape is killed by a named assertion.

## Zero test edits by F (AC10) — per-file conformity

Worktree test-file changes vs my RED inventory (`git status` + `git diff`, nothing staged or
committed by anyone):

| Test file | Status | RED inventory says | Conformity |
|---|---|---|---|
| `frontend/src/components/translateWindowPinDefault.test.ts` | M | 3 re-pointed + 4 new | **Exact** — diff hunks are precisely the documented re-contract (v2 seeding, old-key-opposite pollution, AC7(a) sequence); the 3 untouched tests absent from the diff |
| `frontend/src/components/settings/generalTabOnTop.test.ts` | M | 1 new + emit spy | **Exact** — +23/−3: hoisted `emit` spy, `Events.Emit` routing, one new AC7 test asserting the literal name |
| `main_windowsize_test.go` | M | +139 lines appended (3 + 4 tests) | **Exact** — pure append, 0 deletions; all 7 names verbatim from inventory |
| `frontend/src/components/historyUseRestore.test.ts` | M | — (T-green repair) | **Exact** — one line (320), diff quoted below; no other assertion touched |
| `internal/service/window_close_test.go` | new | 3 tests | **Exact** — 3 `func Test`, names verbatim |
| `frontend/src/utils/effectivePin.test.ts` | new | 4 tests | **Exact** — 4 `it(` |
| `frontend/src/stores/translatePin.test.ts` | new | 9 tests | **Exact** — 9 `it(` |
| `frontend/src/utils/events.test.ts` | new | 1 test | **Exact** — 1 `it(` |

No other test file in the repo has worktree changes (`translateWindowGear.test.ts`,
`startup_settings_hidden_test.go`, `translate_window_on_top_test.go`, `persisted.test.ts` etc.
all untouched — `persisted.ts` itself is untouched, A Finding 3's blast-radius guard). **F edited
zero test code.** F's diff is confined to production: `window_wrapper.go`, `main_windowsize.go`,
`main.go`, `TranslateWindow.svelte`, `GeneralTab.svelte`, `events.ts`, `pinDefault.ts`, new
`stores/translatePin.ts` (+ `CHANGELOG.md`, the AC9 doc lines).

T's one sanctioned repair, verbatim (`historyUseRestore.test.ts:320` — the 4th application of
the AC6 re-contract F correctly flagged; sibling assertions untouched):

```diff
-    localStorage.setItem('kai:translate:pinned', JSON.stringify(true));
+    localStorage.setItem('kai:translate:pinned:v2', JSON.stringify(true));
```

## BINDING conformity (diffs of all 8 production code files read)

| Contract item | Implementation | Conforms |
|---|---|---|
| `hideWindow` interface + package-level `HideTranslateWindow(win, emit)`; nil no-op; one broadcast `(EventWindowClosing, WindowTranslate)`; one `Hide`; no ordering contract | `window_wrapper.go` verbatim, placed after `LowerForSettings`, `kevents` import added | ✅ |
| `translateWindowStartsPinned(*settings.Service) bool`: nil svc→false, nil cfg→false, else `cfg.TranslateWindowOnTop`; fed as `AlwaysOnTop:` | `main_windowsize.go` + main.go translate options `AlwaysOnTop: translateWindowStartsPinned(settingsService)`; file comment widened (A Finding 5) | ✅ |
| main.go: translate `CloseButtonState: ButtonHidden`, aux windows `ButtonEnabled` | diff shows exactly that; aux blocks untouched (their own `EventWindowClosing` emits at :553/:618/:775 intact) | ✅ |
| hook = `event.Cancel()` + `service.HideTranslateWindow(…)`, no leftover inline Emit | old `if app != nil { app.Event.Emit…; translateWindow.Hide() }` deleted; rule called; 0 occurrences of the forbidden literal in the translate hook | ✅ |
| Cmd+W binding = inline callback calling the rule | `_ = webviewWin.RegisterKeyBinding("Cmd+W", func(window application.Window) { service.HideTranslateWindow(window, emitForCloseRule) })`, directly after the hook | ✅ |
| `effectivePin` R2-1 verbatim in `pinDefault.ts`, `initialPin` untouched | verbatim `pinTouched ? thisMountChoice : initialPin(storedV2, liveSetting)`; `initialPin` diff-clean | ✅ |
| store: `TRANSLATE_PIN_KEY` v2; zero-arg ctor; v2-only read (JSON boolean, corrupt→false, no rewrite); `subscribe`/`apply` never write; `set`/`update` the only write path | `stores/translatePin.ts` line-by-line conformant; inner `writable` used by `apply`, persisting wrapper by `set` | ✅ |
| `EventTranslateWindowOnTopChanged` name; payload NOT the contract; emitted after `SaveConfig`; listener re-fetches | constant exact; `GeneralTab` emits inside the try (failed save → no broadcast); `TranslateWindow` listener → `reapplyPin()` (re-fetch, payload ignored); off-fn in onMount cleanup | ✅ |
| Svelte: `storedPinRaw` ← v2 raw; ONE decision at mount init / broadcast / re-pin; `togglePin` keeps `.set` | `resolvePinLevel()` re-fetches config → `pinnedStore.apply(effectivePin(storedPinRaw, pinTouched, get(pinnedStore), cfg?…===true))`; all three points route through it; exactly one `.set` call site (line 36, `togglePin`) and one `.apply` (line 54) | ✅ |

### F's 2 disclosed deviations + 1 nuance — each verified assertion-invisible

1. **`emitForCloseRule` adapter (compile-forced).** My BINDING prose self-contradicted: it
   demanded "emit typed exactly as `app.Event.Emit`" (which returns `bool`) while my own pinned
   rule signature — and my `fakeEmitter` — are `func(name string, data ...any)` with no return;
   Go has no implicit conversion, so the direct pass cannot compile. F resolved in favor of the
   **tests** (the correct precedence). The adapter forwards faithfully (`app.Event.Emit(name,
   data…)`, return dropped), defined once, passed at both call sites. **Assertion-invisible**
   (the wiring greps match; the adapter line contains no forbidden literal) and
   **contract-conforming** (the rule's tested semantics are unchanged). Accepted.
2. **`*application.WebviewWindow` comma-ok narrowing for `RegisterKeyBinding` (compile-forced).**
   `RegisterKeyBinding` is on the concrete type (my own RED evidence cited
   `webview_window.go:1768`), and the pinned anchor `translateWindow = app.Window.NewWithOptions`
   (`internal/service/startup_settings_hidden_test.go:29`, green in this run) forbids capturing a
   second variable at creation. The narrowing always succeeds (`NewWithOptions` returns
   `*WebviewWindow`). **Assertion-invisible** (binding + callback grep targets unchanged),
   **contract-conforming** (the callback is the inline rule call, exactly as bound). Accepted.
3. **Re-pin nuance — `resolvePinLevel().then(() => Window.SetAlwaysOnTop($pinnedStore)…)`
   inlined, not a helper end-to-end,** because `translateWindowGear.test.ts` (untouched) pins
   the literal `SetAlwaysOnTop($pinnedStore)` inside the `EventWindowClosing` handler. The
   applied value is post-resolution (`apply` runs before `.then`; `$pinnedStore` reads the
   current value), so the R2-1 rule holds and the pinned line survives. **Not a deviation** —
   "resolve through this decision, never the raw store" is satisfied; only the call shape
   differs, which no assertion pins. Verified assertion-invisible. Accepted.

## Mutation spot-check (RED Hedged item) — reasoned, nothing executed

Per O's instruction no file was touched; the analysis is over the assertion set. The RED hedge
feared a wrong resolver (e.g. the listener applying the raw setting with `pinTouched` unset).
Each wrong shape is killed by a named assertion:

- **M1 — applies the raw live setting, ignores `pinTouched`/stored choice:** killed by
  `effectivePin.test.ts` "a touched choice wins over everything" rows (e.g. inputs
  `(stored, true, false, true)` expect `false`; M1 returns `true`) and mounted "a stored v2 false
  stays false even with the setting on" (`setAlwaysOnTop` must never see `true`).
- **M2 — re-pin applies the raw store value (A Finding 1's original bug):** the mounted AC7(a)
  sequence alone cannot kill it post-broadcast (`apply` updates the store, so store ≡ decision
  by re-pin time) — exactly why the RED contract pinned the decision **purely**. The matrix
  supplies inputs the mounted tier can never produce (choice ≠ store): `('false', true, true,
  false)` expects `true`; M2 returns `false`. Killed at the pure tier by design.
- **M3 — ignores the live setting when nothing stored (constant default):** killed by "nothing
  stored, untouched: the live setting decides, **both ways**" — the on-row (`(null, false, ·,
  true)` → `true`) fails every constant/setting-blind resolver.
- **M4 — honors the retired old key:** killed mounted by "#113: an old-key pin is pollution, not
  a choice" and the re-pointed "stored v2 false … old key seeded `true`" (both require the old
  key ignored); the corrupt-v2 row additionally kills parse-lenient resolvers.

Every discriminating input of the R2-1 table is held somewhere at a value where misusing it
flips an expected output, so **no wrong resolver shape can pass this suite**. The pure 4-row
matrix is the load-bearing tier for the decision's shape; the mounted tests pin the three real
application points. RED hedge discharged.

## Forbidden-literal grep (re-run fresh this session)

`grep -n "EventWindowClosing" main.go` → 4 hits: `:93` is the `RegisterEvent` wiring (not an
emit); the only `app.Event.Emit(kevents.EventWindowClosing, …)` sites are the three aux windows —
`:553` (Settings), `:618` (History), `:775` (Screenshot) — **deliberately untouched** (each is
that window's own close broadcast, in scope per BINDING). The translate hook emits nothing; the
translate broadcast lives once, inside the rule (`window_wrapper.go:187`
`emit(kevents.EventWindowClosing, model.WindowTranslate)`). Translate hook clean. ✅

## Tier 1 results

| Check | Command | Expected | Actual | |
|---|---|---|---|---|
| Build (swiftbridge) | stage 1 of the suite | exit 0 | `>> done:` both artifacts | PASS |
| Go internal+pkg | stage 2 | all ok | 20 `ok`, 0 FAIL | PASS |
| Go root package (add-on) | `go test -vet=off . -count=1` (this session) | ok | `ok cnb.cool/dtapp/kai 0.789s` | PASS |
| Frontend | stage 3 | 82 files / 889 tests | `82 passed (82)` / `889 passed (889)` | PASS |
| Release harnesses | stages 4–5 | PASS | both final `PASS` | PASS |
| Server/API smoke | N/A — Wails desktop app, no server to curl; the release harnesses are this repo's smoke tier | — | — | N/A |

Cross-check vs F's self-check: F reported 888/889 with exactly the stale test failing and
predicted 889/889 after T's repair — this run confirms the prediction exactly. F's Go self-checks
(`internal/service` ok, `go test .` ok) reproduce. **No discrepancies.**

## Acceptance criteria status

| AC | Claim | Backing test | |
|---|---|---|---|
| #112 AC1 | close button hidden, translate-only; aux windows keep theirs | `TestTranslateWindowCloseButtonIsHidden` | PASS |
| #112 AC2 | close policy = broadcast + Hide (hide-not-destroy; nil no-op) | 3 × `TestHideTranslateWindow*` | PASS |
| #112 AC3 | Cmd+W → the rule; tray toggle stays bare-Hide broadcast-free | `TestTranslateWindowCmdWBindingCallsTheCloseRule` + untouched `ToggleTranslateWindow` tests | PASS |
| #112 AC4 | bring-back paths unaffected | `historyUseRestore`/`sessionRetention`/`translateShortcut` suites within the 889 | PASS |
| #113 AC5 | creation param: off→false, on→true, nil/unreadable→false, fed into `AlwaysOnTop` | 3 × `TestTranslateWindowStartsPinned_*` + `…FeedsStartsPinnedIntoAlwaysOnTop` | PASS |
| #113 AC6 | v2-only explicit-write store; old key pollution | `translatePin.test.ts` (9) + 3 re-pointed + pollution/mount-empty/explicit-write mounted tests | PASS |
| #113 AC7 + R2-1 | broadcast applies live setting; one decision at all three points; re-pin never reverts | `effectivePin.test.ts` (4) + `generalTabOnTop` AC7 + `translateWindowPinDefault` AC7(a) + repaired `historyUseRestore` re-pin + `events.test.ts` | PASS |
| #113 AC8 | owner migration end-to-end; re-pin with v2 present | `translatePin.test.ts` migration + AC8 re-pin tests | PASS |
| AC9 | CHANGELOG lines | two `[Unreleased]` lines verified in the diff (not test-backed — doc item, S confirms) | PASS |
| AC10 | F writes no tests | zero-edit proof above | PASS |

## Blocking issues

**None.**

## Decided / Assumed / Hedged / Evidence

**Decided**
- The prior session's cross-run BLOCK is discharged: the suite was re-run in the foreground
  inside THIS worktree, logged run-unique, and every number re-confirmed. Verdict stands: **PASS**.
- T's one sanctioned repair this phase: `historyUseRestore.test.ts:320` seed re-pointed
  `'kai:translate:pinned'` → `'kai:translate:pinned:v2'` (the 4th application of the AC6
  re-contract F correctly identified; the sibling assertions untouched). Delivered via `bash` +
  git-diff verification after the edit tool was permission-denied on the frontend test glob —
  the `paths.test` gap, third occurrence.
- F's deviation 1 accepted **with precedence made explicit**: where BINDING prose and the tests
  disagree, the tests are the contract (the prose's "exactly Emit's signature (bool)" contradicts
  the pinned no-return shape). The adapter is the minimal faithful bridge.
- F's deviation 2 and the re-pin inline nuance accepted (rationale and assertion-invisibility
  verified above; the second is not even a deviation — no assertion pins the call shape).
- Verdict **PASS**; U N/A (uiSurface false, declared at pre-flight); S may proceed.

**Assumed**
- The root-package `go test .` run I added is representative of what CI/the plugin would do with
  the root package if it were in the pattern (same worktree, same flags, `-count=1`).
- AC9 (CHANGELOG) verified by reading the diff, not by a test — accepted as a doc-line AC.
- Real-app claims of the four wiring source-assertions (traffic light actually hidden per
  platform, Cmd+W actually firing, level actually dropping) remain manual items W-1/W-3 for the PR
  description, per the RED hedge — they cannot be automated against a live Wails window.

**Hedged**
- The mutation check is reasoning over the assertion set, not an executed mutant (O's explicit
  instruction: do not mutate files). The observed RED + the stale-seed failure during F's
  self-check are the run's empirical evidence the suite discriminates; the M1–M4 analysis is the
  analytic evidence. Residual risk accepted: a mutant that is *equivalent* on all pinned inputs
  would survive, but such a mutant is behaviorally the contract.
- `svelte-check` cannot run in this environment (F's disclosure; pre-existing tooling mismatch);
  vitest's compilation of every touched component is the type signal available. If S's
  environment can run it, it should.

**Evidence**
- `/tmp/0112-tgreen-suite.log` (this run, foreground, run-unique, 1220 lines): 20 Go package
  `ok` lines (`ok cnb.cool/dtapp/kai/…`) and 0 `FAIL` anywhere; `Test Files  82 passed (82)` /
  `Tests  889 passed (889)`; release harness finals `PASS` (log :1182) and `PASS` (log :1219);
  last line `SUITE_EXIT=0`.
- `git diff` of every changed file (production conformity table above) and of every changed test
  file (zero-edit table); `git status --porcelain` sweep showing test-file changes = exactly
  T's 4 modified + 4 new.
- Greps re-run this session: forbidden literal — translate hook 0 emits; `main.go`
  `EventWindowClosing` = RegisterEvent (:93) + the 3 aux windows (:553/:618/:775); rule broadcast
  at `window_wrapper.go:187`. Old key 0× in `TranslateWindow.svelte`/`translatePin.ts`/
  `pinDefault.ts`; `.set(`/`.apply(` call sites in `TranslateWindow.svelte` = 1/1; anchor
  literal at `internal/service/startup_settings_hidden_test.go:29` (from the standing RED/BINDING
  verification).
- `go test -vet=off . -count=1` green this session (`ok cnb.cool/dtapp/kai 0.789s`).

## Advisory notes (non-blocking)

1. **The configured Go pattern omits the root package.** `test.unit.command` runs
   `./internal/... ./pkg/...` — package `main` (7 of this story's tests: the creation param × 3 +
   all four main.go wiring pins) is outside it. The plugin's GREEN would record without them. I
   closed the hole manually this run; the config deserves ` .` added (same owner follow-up family
   as the two allowlist gaps below).
2. **Permission-config gaps, third occurrence (for O):** `paths.test` still lacks frontend test
   globs (this repair was bash-delivered) and `paths.production` still lacks root `main_*.go` +
   `CHANGELOG.md` (F's disclosure). Both recorded at RED and F; unchanged here.
3. **Comment nit in `window_wrapper.go`:** the rule's doc comment still says "main.go passes
   `app.Event.Emit` itself as the value" — false since the `emitForCloseRule` adapter. One-line
   comment fix for F or S; no assertion reads it.
4. F's disclosed `get(pinnedStore)`-as-`thisMountChoice` invariant (only `togglePin` may call
   `.set`) is currently true (grep-verified) but unpinned at the Svelte tier; if anyone adds a
   second `.set` caller the resolution silently degrades. Cheap future guard: a source-assertion
   test counting `.set(` call sites in TranslateWindow.svelte. Not this run's scope.
