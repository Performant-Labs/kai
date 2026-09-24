# Design — issue #9: Result pane bound to active engine (multi-engine fan-out kept)

Bead `kai-5fb.1` · role Designer · rigor `second-opinion` · `uiSurface: true`.
Part of #6. Follows the format and depth of `docs/design/issue-0008-primary-engine.md`.

## Problem

The translate window renders one card per enabled engine (`TranslateWindow.svelte:666-769`
`{#each activeEngines as e}` inside the result section), each with its own expand/collapse
(`expanded` state, lines 151-165, `DEFAULT_EXPANDED = 2`), per-card copy button (lines 683-705)
and a "copy all engines" button in the pane header (lines 616-641). That fragments attention and
makes the window tall. Multi-engine fan-out is still useful machinery — it just shouldn't *be*
the UI.

The result pane must show exactly ONE result — the **active engine's** — while the backend
`TranslateMulti` fan-out (`internal/translate/service.go:181`) keeps running every enabled engine
and streaming each result through `EventTranslateResult` (`internal/events/events.go:29`) exactly
as today.

Issue #8 (merged as 743d0b5) already delivered everything this design consumes:
`settings.default_engine` (primary), `EngineWrapper.PrimaryTranslateEngine()` (authoritative
backend resolution), and the frontend mirror `resolvePrimaryEngine(lastUsedKey, defaultEngine,
engines[])` (`frontend/src/utils/resolvePrimaryEngine.ts:41`) with `lastUsedEngine`
(`kai:translate:lastEngine` via `persisted()`, `TranslateWindow.svelte:133-135`),
`defaultEngine` state, `allEngines` (`GetAllEngines` shape) and the `resolvedPrimary` derived
(lines 144-146). This design does **not** re-derive the resolution rule; it consumes
`resolvedPrimary` and `setLastUsedEngine` (`TranslateWindow.svelte:381-383`).

## Decisions

### 1. The active engine = `resolvedPrimary` (no new selector state)

The active engine is exactly the `resolvedPrimary` derived introduced by #8
(`TranslateWindow.svelte:144-146`):

```
activeEngine := resolvePrimaryEngine(LAST_ENGINE_KEY, defaultEngine, allEngines)
               = lastUsed ?? primary(default_engine) ?? first enabled translate engine ?? ''
```

Inputs to `resolvePrimaryEngine` (all already owned by `TranslateWindow`):

1. `LAST_ENGINE_KEY = 'kai:translate:lastEngine'` — the function reads the **real**
   `localStorage` itself (`resolvePrimaryEngine.ts:49-52`, JSON string). It wins if the stored
   name is a present, enabled, translate, supported engine in `allEngines`.
2. `defaultEngine` — `settings.default_engine` (settings.json), read in `onMount` via
   `GetConfig()` (lines 304-314). Wins if valid + enabled.
3. `allEngines` — `GetAllEngines()` list (id order; the "first enabled" ordering), refetched on
   `EventEnginesChanged` (lines 281-283, 363-376).

Because all three inputs are already reactive (`$derived.by`, #8 lines 144-146), `activeEngine`
re-resolves automatically when the user switches the dropdown (writes last-used via
`setLastUsedEngine`), when `default_engine` changes, or when an engine is enabled/disabled in
settings. There is **no second selector state**: the result-pane dropdown is a *view* over the
same `lastUsed`/`primary` chain the settings star feeds. This is the pinned decision "default =
primary from #8's `default_engine` + last-used persistence from #8" — switching in the pane is
exactly the last-used write #8 anticipated ("由下一 issue 的结果区引擎选择器调用",
`TranslateWindow.svelte:378-380`).

The dropdown's displayed value is `activeEngine || firstEnabledName`: if `allEngines` has at
least one enabled translate engine but `resolvePrimaryEngine` returned `''` (degenerate case),
the select shows the first enabled engine so the pane never points at nothing. In practice this
branch is empty because rule 3 of `resolvePrimaryEngine` already returns the first enabled
engine; the `|| firstEnabledName` is a defensive no-op kept so a future change to the resolution
rule cannot leave the select unbound. When **zero** engines are enabled, the select is `disabled`
and the pane keeps the existing "no active engine" empty state (lines 645-662).

### 2. Result-pane header: engine dropdown

The header row (lines 611-643) becomes:

```
[ Result label ]                    [ ▾ engine select ] [ ● ● ● status dots ] [ copy ]
```

The select reuses the existing native-select approach of the language bar —
`class="u-field u-select u-engine-select px-3 py-2 text-sm"` with `{#each activeEngines as e}
<option value={e.value}>{engineName(e.value)}</option>{/each}` (pattern: lines 463-472 and
495-504; the `u-engine-select` CSS class already exists at `app.css:500-501`). No custom
dropdown widget.

- `value={activeEngine || firstEnabledName}` (controlled, **not** `bind:value` — the value is
  derived from last-used/primary; a bare bind would fight the re-resolution).
- `onchange`: (1) `setLastUsedEngine(e.currentTarget.value)` — the existing #8 function
  (lines 381-383) writes through the `persisted` store → real `localStorage`; (2)
  `edited.clear()` (see §3); (3) `adjustWindowHeight()`.
- Options are the `activeEngines` list (translate engines only — the pane only ever shows a
  translate result). An option for an engine that is in the list but currently `enabled === false`
  (per `allEngines`) is rendered `disabled` with the name suffixed
  `t('translate.engineDisabled')`; such an option can exist right after a settings toggle, before
  the `EventEnginesChanged` refresh lands.
- `aria-label={t('translate.engine')}`, `title={t('translate.engine')}`.
- While `activeEngines.length === 0` the select is hidden and the pane shows the existing
  "no active engine" empty state (unchanged, lines 645-662).

### 3. State: what switching means (re-render + reset manual edits)

Current state (all `TranslateWindow`): `results: Record<string, TranslateResult>` keyed by engine
name (lines 90-91, populated per-event at 257-264); `expanded: Record<string, boolean>`
(151-165); `loading` (92); `dotCount` marquee (115-125); the per-card `isOpen` local inside the
inline card markup.

After this change:

- `results` is **kept** as a full per-engine map: fan-out is unchanged, results still arrive
  per engine and the status dots need the whole map. It is the *data* the fan-out produces.
- **What is deleted:** the `expanded`/`toggleExpand`/`DEFAULT_EXPANDED` machinery
  (lines 103, 151-165) and the per-card expand/collapse buttons (lines 677-753); the whole
  `{#each activeEngines}` card list (lines 664-769); the header copy-all button (lines
  616-641). (The `TranslateCard.svelte` component itself is kept — ScreenshotWindow still uses it,
  `ScreenshotWindow.svelte:14,409-415`.)
- **What is kept:** the results map, the `EventTranslateResult` subscription (257-264), the
  `EventEnginesChanged` → `loadEngines()` (281-283), `adjustWindowHeight` + `resultH` measurement
  (194-253, driven by the existing `$effect` at 168-177 — the effect's `results` dependency now
  re-measures for whichever engine is active), the empty/loading/failed content states, and the
  pane's dynamic height.
- **Manual-edit reset:** the active result's text becomes a single editable field,
  `let edited = $state<Map<string, string>>(new Map())` (keyed by engine name). Displayed text =
  `edited.get(activeEngine) ?? activeResult?.result ?? ''`. "Switching engines re-renders and
  resets manual edits" means: on every `onchange` the new engine starts from its stored result —
  `edited.clear()` drops the previous engine's edits, so the incoming view is always the
  unedited result of the engine just selected (its edits, if any, were discarded). This is the
  pinned "manual edits reset" decision; there is no per-engine edit memory.
- `doTranslate()` (lines 405-429): additionally does `edited.clear()` next to `results = {}`
  (line 408). A new fan-out starts clean; old edits of the previous batch are meaningless against
  new results.
- Clear input / window close: `clearInput()` (449-454) and the `EventWindowClosing` handler
  (270-278) also do `edited.clear()` alongside their existing `results = {}`.

### 4. Status dots: states, colors, data source

Row of one dot per enabled translate engine, in `activeEngines` order, in the header to the left
of the copy button. Each dot: `title`/`aria-label` = engine name; the active engine's dot carries
an accent ring (`ring-2 ring-[var(--app-accent)]`) so the select's choice is visible at a glance.

Per-engine state, derived from the fan-out's existing signals (no new backend event, no new
polling — the design deliberately stays inside the data `TranslateMulti` already produces):

| dot | condition (against `results` + `loading`) | color |
| --- | --- | --- |
| pending | `loading === true` && no entry in `results[engine]` yet | `var(--app-muted)` (border grey), matching the `kai-loading-bar` track (app.css:581-586) |
| done | `results[engine]?.result` is a non-empty string | `var(--app-accent)` (same token the loading bar's light band uses, app.css:589-598) |
| failed | fan-out finished for this engine with no usable result | `var(--app-danger)` (the existing failure color, e.g. `TranslateCard.svelte:60`) |

**Critical honesty point — the failed state's data source:** a failed engine **emits no event at
all**. `TranslateMulti` logs the error and returns on error without emitting
(`internal/translate/service.go:202-207`: `slog.Error` + `analytics.Error` + `return`); only
successful results go through `EventTranslateResult` (`events.go:29`). So the frontend cannot
learn "engine X failed" from a result map — a failed engine is simply *absent* from `results`.
The failed state is therefore derived: `failed(eng) = !loading && !results[eng]?.result`. The
existing 15 s fallback timer that clears `loading` when **zero** engines returned
(`TranslateWindow.svelte:424-427`) is extended from "zero results" to "any pending engine
remains":

```
setTimeout(() => {
  if (activeEngines.some((e) => !results[e.value]?.result)) loading = false;
}, 15000);
```

Consequences, stated plainly (this is the design's only real ambiguity in the issue text, and it
is a known cost of keeping fan-out unchanged). Failure modes, with their exact timing:

1. **Failed engine among several** (the main case): each other engine's success flips
   `loading = false` (line 260) as it lands, so the failed engine's dot flips *pending →
   failed* at that moment — real signal, no wait. A *slow* (not failed) engine simply stays
   pending until its own result arrives or the 15 s backstop fires.
2. **Failed sole engine**: nothing resolves `loading` (the timer predicate in §4 requires only
   *a pending* engine, which is trivially true), so the dot reads *pending* — the pane body's
   loading marquee — for the **full 15 s**, and the failed dot appears at t = 15 s. This is the
   accepted lag: the 15 s fallback was already the honest floor for "all engines dead" in the
   current per-card UI (cards show the loading block until it fires), so this design adds no new
   delay to any state, it only re-presents the same backstop through the dot.
3. **Slow engine misread as failed**: bounded, not unbounded. A per-engine goroutine carries a
   30 s context (`service.go:139`) and the goroutine gives up on it; no engine outlives ~30 s.
   If the slow engine is the only one *and* the user re-translates before 15 s, `doTranslate()`
   resets `results = {}` (line 408), so the stale result cannot be misread as the new attempt's
   failed state. If no new request is made, the engine's own result (≤30 s) or the 15 s backstop
   ends the pending state — either way it flips to its true state.
4. **Stale events from an earlier request**: `EventTranslateResult` has **no request id**
   (`model.go:59-69`; the only per-request scoping is `results = {}` at the start of
   `doTranslate`, line 408). If the user re-translates while an earlier attempt is still
   in-flight, a late result for engine X from the old attempt can overwrite the new attempt's
   pending/done state in `results` (and a failed-engine absence can be filled with stale text).
   This is a pre-existing race in the current per-card UI (same `results` map, same no-id
   events), not one this design introduces; the dots inherit exactly the same exposure the cards
   have today. A request-id on the result event would fix it backend-side (`service.go:209` +
   `model.go:60`) — a fan-out-signal change, hence out of scope per "kept", and not required by
   this issue's "real results" bar. If it is ever fixed, it is a separate bead.

So: the *failed* dot is a real *absence* signal in case 1 (arrives at the moment a sibling
resolves), an *inference* in case 2 (bounded by the existing 15 s backstop, user-accepted), and
cases 3–4 bound the two remaining misread modes. This satisfies "dot state derives from real
per-engine results including failures" without a backend change: every dot state is a pure
function of the fan-out's actual output (presence/absence of `results[eng]` + the existing
backstop), never invented state. A real failure *signal* (an `EventTranslateEngineFailed` emit in
the goroutine's `err` branch, `service.go:202-206`) would make case 2 instant and would be the
honest fix — it is deliberately deferred as a separate bead because the issue pins the backend as
untouched; deferral is sound *only* because case 2's cost is bounded by an existing, already
user-accepted backstop (see case 2 above) and case 1 — where failures are actually informative —
is already real-time.

While `loading` is true, every not-yet-returned engine's dot is *pending* (grey) — the per-engine
granularity is "not done yet" vs "done". The existing `dotCount` marquee (lines 115-125) stays
only in the pane body for the active engine; it is not used in the dots.

### 5. Pane body: the single active result

Inside the existing result section (height machinery untouched, lines 610, 644):

```
{#if activeEngines.length === 0}
  (existing "no active engine" empty state, lines 645-662 — unchanged)
{:else if loading && !results[activeEngine]}
  (existing per-card loading block: "Translating" + kai-dots + kai-loading-bar,
   lines 758-764 — unchanged, just scoped to the active engine)
{:else if activeResult?.result}
  (editable result — see below)
{:else}
  (existing failure text: t('translate.failed'), e.g. a grey/`--app-danger`
   "Translation failed" line, matching TranslateCard.svelte:59-62)
{/if}
```

where `activeResult = $derived(activeEngine ? results[activeEngine] ?? null : null)`.

**When the active engine failed:** `results[activeEngine]` is `undefined` (no event was ever
emitted, see §4) and `loading` is false → the pane body shows the failed state
(`t('translate.failed')`, `--app-danger`). No re-translate button, no retry: retrying is the user
re-hitting the existing Translate button (which re-runs the full fan-out). If the user switches
to another engine in the dropdown, that engine's done/failed state renders independently.

**When the active engine is disabled:** disabling a translate engine emits
`EventEnginesChanged` (EnginesTab `toggleEngine`), the window re-runs `loadEngines()`
(281-283) → `allEngines` updates → `resolvedPrimary` re-resolves. If the disabled engine was the
active one, `activeEngine` **automatically falls through** the #8 chain (last-used invalid →
primary → first enabled) and the pane re-renders to the next engine. The disabled engine's
**stale result stays in `results`** (harmless — the dots are derived from `activeEngines`, so no
dot renders for it; the copy button copies only the active result). The dropdown still lists it
(disabled option, §2) so the user sees why the pane moved. No `results` entry is evicted,
matching the current semantics where results survive engine-list changes (the EventEnginesChanged
handler today does not touch `results`, line 281-283).

**The single result, editable:** one `textarea` reusing the input textarea's class
(`min-h-[120px] resize-none bg-transparent p-4 text-base leading-relaxed outline-none`, line 567)
bound to `edited.get(activeEngine) ?? activeResult?.result ?? ''`; on change it writes
`edited = new Map(edited).set(activeEngine, value)`. Placeholder when the active result has an
empty `result` (empty-engine-success) but no edit yet. Below it: the phonetic line when
`activeResult.phonetic` (`u-muted text-xs`, pattern line 679) and the existing `u-result-card`
chrome around the whole block (app.css:571-578). This keeps the pane a card — just one.

### 6. Header copy button: active engine only

The existing header button (lines 616-641) is kept in place but repointed: it disappears when
`Object.keys(results).length > 0` is false (i.e. only when the **active** engine has a result)
and copies `edited.get(activeEngine) ?? activeResult?.result` — i.e. the text currently on
screen, edits included (that is what "copy the result" means once the result is editable). The
label stays `t('translate.copy')` (the old "copy all" behavior had no distinct i18n key; the
`translate.copy` key is reused, its old join-all meaning is simply gone). The per-card copy
buttons (lines 683-705) are deleted with the cards.

### 7. i18n keys (en-US + zh-CN)

New keys, all under the existing `translate` namespace (`en-US.ts:46`, `zh-CN.ts:43`):

| key | en-US | zh-CN | used by |
| --- | --- | --- | --- |
| `translate.engineActive` | 'Active engine' | '当前引擎' | dropdown `aria-label`/`title` (existing `translate.engine` is the generic 'Engine' label, line 49 — the header's left label stays `translate.result`) |
| `translate.engineDisabled` | 'disabled' | '已停用' | disabled `<option>` suffix in the dropdown |
| `translate.enginePending` | 'Translating…' | '翻译中…' | dot title/aria for a pending engine |
| `translate.engineDone` | 'Done' | '已完成' | dot title/aria for a done engine |
| `translate.engineFailed` | 'Failed' | '已失败' | dot title/aria for a failed engine |

Reused, **unchanged**: `translate.copy` (53/50), `translate.result` (54/51),
`translate.failed` (59/56, the failed state in the pane body), `translate.noActiveEngine`
(57/54), `common.loading` (43/40), `common.copied` (44/41), `translate.noResult` (55/52,
optional placeholder for the empty-edit case).

No new `log` keys: switching the dropdown is a local-state operation with no backend call, so
there is nothing that can fail to log (contrast `setLastUsedEngine` — localStorage writes fail
silently inside `persisted`'s subscriber, `persisted.ts:14-19`, by design).

## UI pattern verdict

**Engine dropdown in the result-pane header — REUSE.** It is a native `<select>` using the
exact class combination the app already uses for its language dropdowns:
`u-field u-select` + the `u-engine-select` width class (`app.css:176-215`, `app.css:500-501`)
with the same `{#each}`-of-`<option>` body and per-option `langName(...)` label pattern as the
language selects at `TranslateWindow.svelte:463-472` / `495-504`. The only deltas are the option
source (`activeEngines` + `engineName` instead of `languages` + `langName`) and a `disabled`
attribute on options for disabled engines — `:disabled` styling on `.u-select` already exists
(`app.css:192-195`); extending the same rule to `<option>` is CSS, not a new control. No custom
dropdown/menu component exists in the app today (the only "dropdowns" are these `<select>`s and
the settings tabs), so nothing else to claim.

**Per-engine status dots — REUSE** (colors + placement). Each dot is a plain
`<span class="h-2 w-2 rounded-full">` with a `title`/`aria-label` — the same
span-plus-title icon-button idiom as the star in the #8 engines tab
(`EnginesTab.svelte:543-553`). The three colors are all tokens the app already uses for exactly
these states: pending = `var(--app-muted)`/`--app-border` (the loading-bar track,
`app.css:581-586`; the `u-muted` class at `app.css:55`), done = `var(--app-accent)` (the
loading-bar light band, `app.css:589-598`; the `u-icon-btn--active` fill at `app.css:505-508`),
failed = `var(--app-danger)` (the existing failure text color, `TranslateCard.svelte:60`). The
dot row's placement in the header is the same `u-border-b ... px-3 py-2` header row already used
by the pane (lines 611-643) and by the input card (509). **Honest caveat for the gate:** no
literal *status-dot* widget exists in the codebase yet — this is the first one — but it is
composed entirely of existing classes, tokens and placement patterns, no new widget, no new
interaction, no new CSS class (inline `w-2 h-2 rounded-full bg-[var(--app-…)]` utilities, as the
app already mixes in Tailwind utilities, e.g. lines 464, 520). Verdict REUSE, on the record.

**Delete-card / expand-collapse / copy-all controls — REUSE.** These are deletions, not
additions: the card markup (`TranslateWindow.svelte:664-769`), the `expanded`/`toggleExpand`
state (151-165) and the header copy-all button (616-641) are removed; nothing new is introduced
to replace them except the single active-result block (existing `u-result-card`, app.css:571),
the editable textarea (existing input-textarea class, line 567), and the repointed copy button
(existing `u-icon-btn u-no-drag` + copy SVG, lines 617-641, as-is). Deletion introduces no new
UI pattern by definition.

**OVERALL: REUSE**

## Out of scope

- **Two-column layout — #10.** This design works entirely inside the current stacked layout
  (fixed language bar + input card, then one result section, `main` at lines 458-773). Any
  left-right split of the result pane belongs to #10.
- **Backend changes to `TranslateMulti`** — no new failed/done events, no `done` counter in
  `TranslateMultiResult`; the fan-out is byte-for-byte unchanged (issue: "kept"). A real failure
  signal (`EventTranslateEngineFailed` in the goroutine's error branch, `service.go:202-206`, or
  an error/result-scoping field on `TranslateResult`) is the honest fix for the case-2 lag in §4
  and for the no-request-id stale-event exposure — both tracked there as deliberately deferred to
  a separate bead, with the deferral's soundness argued there.
- No new settings fields, no changes to `default_engine`/`PrimaryTranslateEngine`/#8 semantics.
- No re-translate/retry button in the failed state; no per-engine edit memory; no ScreenshotWindow
  changes (its per-engine `TranslateCard` list stays — that window is a different surface and is
  not named by this issue).

## Tests (t-red; no mocks — real files/SQLite, real loopback HTTP, real jsdom)

Matches issue #9 "Tests" (a)/(b)/(c); (c) is verified in ui-walkthrough, not in this list.

- **(a) `internal/service/engine_wrapper_primary_test.go` — extend the existing #8 file**
  (real configstore on `t.TempDir()`, engine registry against `httptest.NewServer` loopback with
  each engine `Endpoint` pointed at it). New cases: `PrimaryTranslateEngine()` returns the
  last-used engine when one is supplied for the pane's resolution chain — i.e. add a
  last-used-accepting variant (the service-level resolution the pane relies on:
  given `lastUsed` + `default_engine` + engine list → expected name) covering: last-used wins;
  last-used disabled → primary; primary invalid → first enabled; all empty → `''`. Fails before
  the implementation if the wrapper gains no such entry point / returns the primary instead of
  the last-used.
- **(b) `frontend/src/utils/resultPane.test.ts` (new file, vitest, jsdom with real
  `localStorage` and real timers, same harness as `resolvePrimaryEngine.test.ts`)** for the new
  pure result-pane logic module the implementation must extract:
  - `activeEngineFor(lastUsed, defaultEngine, engines)` = `resolvePrimaryEngine(...)` + the
    `'' → first enabled` defensive fallback (asserts the dropdown is never unbound);
  - per-engine **status-dot mapping** `(engines[], results{}, loading) → 'pending'|'done'|'failed'
    per engine`, including: pending while `loading` and no entry; done on non-empty `result`;
    failed only when `!loading` and no non-empty result; a failed *active* engine with a second
    engine succeeding → `loading` false, active dot failed (immediate, case 1); a failed *sole*
    engine → dot pending until the 15 s fallback flips `loading` false, then failed (case 2); and
    the extended 15 s fallback predicate ("any engine still without a result") evaluated with
    real `setTimeout` + jsdom timers, not a mocked timer.
  - edit-reset contract: a function taking `(edited, previousEngine, nextEngine, results)` →
    new `edited` map, asserting the previous engine's edits are dropped and the incoming engine
    starts from its stored `result`.
  Fails before implementation because `resultPane.test.ts` imports modules/functions that do
  not exist yet (the result-pane logic is currently inline in `TranslateWindow.svelte`).
- **(c) ui-walkthrough** (no code here): fan out a translation with ≥2 engines → header shows the
  engine dropdown (active = last-used/primary) + one dot per engine, pending→done/failed;
  switching engines re-renders the body and a prior edit is gone; a disabled active engine
  auto-falls through; copy copies the active engine's text.

No mocks anywhere: SQLite via `t.TempDir()`, engines against loopback `httptest` through
`Endpoint`, browser storage/timers are real jsdom.

## Preflight result (recorded, not fixed — per bead instructions)

The `pipeline_preflight` tool / `node .opencode/plugins/coding-pipeline/runner.ts` is not part of
my tool surface in this session, so I did not run the preflight (the bead says to run it only if
the command is directly available; it is not). The known environment rows from the #8 design's
preflight record remain the standing baseline — **recorded, not fixed**: role-models unresolved
(orchestrator, tester, feature-implementor, architecture-reviewer, spec-auditor, designer,
playwright-ui-walkthrough); toolchain:swiftc (not installed on Linux host);
private-registry-auth (npm E403 @performant-labs); dual-review-interface (no reachable
dual-review.sh); gate-model-reachable (no gate provider); playwright-mcp (not configured in
opencode.json — this also means the later ui-walkthrough phase (c) will be blocked in the same
environment and will need a human waiver at run time). This design itself introduces no new
toolchain or wiring requirements beyond what #8's record already covers.
