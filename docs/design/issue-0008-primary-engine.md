# Design — issue #8: Primary engine setting (`default_engine`)

Bead `kai-bhw.1` · role Designer · rigor `second-opinion` · `uiSurface: true`.
Part of #6 (result pane needs a deterministic engine to bind to on first paint).

## Problem

No primary/default engine exists. The only "defaults" today are which engines are
enabled out of the box (`internal/engine/engine.go:98` `defaultEngineNames`) and the
default languages (`default_from` / `default_to`). The translate window currently runs
every enabled engine (`activeEngines = engines.filter(kind === 'translate')`,
`TranslateWindow.svelte:124`) — there is no notion of *which* engine is the one the
user cares about first.

## Decisions

### 1. Config field `default_engine` (persisted via the existing SaveConfig flow)

- Add `DefaultEngine string` to `settings.Settings`
  (`internal/settings/service.go:19`, json `default_engine`). Empty string = unset.
  It is an **engine name** (`value`, e.g. `"google"`), never the auto-increment id:
  ids are configstore-local and `GetEngines`/`GetAllEngines` already key by `value`.
- `setDefaults()` (`service.go:266`) and `writeConfig()` (`service.go:333`) each get a
  `default_engine` line, following the `default_from`/`default_to` pattern (lines 270-271,
  341-342). No viper default is needed to force a value: the field is opt-in, empty is valid.
- `ConfigWrapper.SaveConfig` (`internal/service/config_wrapper.go:79-95`) gets
  `cur.DefaultEngine = cfg.DefaultEngine` added to its existing copy list (alongside
  `Language`/`Theme`/`DefaultTo`/`DefaultFrom`/`Hotkeys`/`TTS`/`ExecKeys`/
  `AutoClipboard`/`CopyKeySnapshot`/`AnalyticsEnabled`). No event emission is needed
  for it: the settings UI is in-process and re-reads after save; the translate window picks
  it up on next load / on `EventEnginesChanged`.
- No `DefaultSettings()` entry (empty = fallback rule applies) — consistent with
  `AutoClipboard`/`CopyKeySnapshot` which are also zero-value defaults.
- The Wails-generated `frontend/bindings/.../settings/models.ts` `Settings` interface gains
  `default_engine: string` (regenerated, not hand-edited).

Resolution (backend, new helper on `EngineWrapper`, e.g. `PrimaryTranslateEngine() string`):

```
1. cfg := settings.Get(); name := cfg.DefaultEngine
2. if name != "" && engine named exists in GetEngines() && kind=='translate' && enabled
     -> return name
3. fallback: first enabled translate engine in GetEngines() order
   (GetEngines is ordered by configstore id, engine_wrapper.go:149-177)
4. none enabled -> "" (translate window keeps its current no-active-engine handling)
```

Steps 2-4 are the spec rule: *unset / invalid name / engine later disabled -> first
enabled translate engine from `GetEngines`*. "Invalid" covers: name empty, name not
present in the engine list, kind not `translate`, or engine present but `enabled == false`
(the disabled case is the spec's "engine later disabled" — it must fall back, not error).

The frontend mirror of this rule lives in TranslateWindow (see 4) so first paint needs no
backend round-trip ordering guarantee; both sides implement the identical rule so they
can never disagree on *which engine is primary*.

### 2. Settings UI — "set as primary" affordance on enabled translate engines

Location: `EnginesTab.svelte`, inside the per-engine row of the translate group
(`u-list-item`, lines 489-518). Today each supported row is:
`[name button (flex-1)] [u-switch]` or `[name button] [unsupported span]`.

Design: add a star **inside the same `.u-list-item` flex row**, before the name button:

```
{#if e.kind === 'translate'}
  <button
    class="u-icon-btn u-icon-btn--sm"
    class:u-icon-btn--active={primaryEngine === e.value}
    disabled={!e.enabled}
    aria-label={t('settings.engineSetPrimary')}
    title={t('settings.engineSetPrimary')}
    onclick={() => setPrimary(e)}>
    ★
  </button>
{/if}
```

Interaction (immediate, like the existing enable toggle — not part of the form's Save button):

- `setPrimary(e)`: if the engine is already primary, clicking clears it (un-set).
  Otherwise `await SaveConfig({ ...cfg, default_engine: e.value })` (the exact
  `persistLangs` read-modify-write shape, `TranslateWindow.svelte:321-328`), then
  `emitEvent(EventEnginesChanged)` so the translate window re-resolves, and refresh the
  star state (re-read via `GetConfig()` or keep a local `primaryEngine` state seeded at
  load from `GetConfig().default_engine`).
- Failed `SaveConfig` → `Dialogs.Error` with `settings.engineOpErrorTitle` + `parseErr`,
  same as every other mutation in this file (lines 310-315); local star state rolls back.
- The affordance is rendered **only for `kind === 'translate'`** engines (spec: "on
  enabled translate engines"); OCR rows are untouched. `disabled` (and dimmed, reusing the
  existing `!e.supported` muted treatment, line 498) when `!e.enabled` — you cannot make a
  disabled engine primary. A star on a disabled row is also never rendered active: the
  resolved primary is always an *enabled* engine (see 3), so `disabled` is purely an input
  affordance state.
- When the primary engine is later disabled (its u-switch turned off), the backend
  fallback rule takes over automatically; the settings star should visually demote — on
  `loadEngines()` after a toggle, if `cfg.default_engine` names a now-disabled engine,
  the star is not shown active (derived from `enabled`), without erasing the saved value.
  We deliberately do **not** auto-rewrite `default_engine` to the fallback: the user may
  re-enable the engine and the star should come back (spec says fallback *resolves*, not
  *overwrites*). If the list ever contains zero enabled translate engines, no star is active.

### 3. Fallback rule (one authoritative implementation + a documented frontend mirror)

`unset / invalid / disabled -> first enabled translate engine from GetEngines`.
The rule is **not** currently "in one place": the Go helper (`PrimaryTranslateEngine()`,
see 1) is the authoritative source for backend consumers and tests (b)/(c); the frontend
resolution helper (see 4) re-implements the same predicate against its own in-memory
engine list so first paint does not race a second backend round-trip. Two implementations
can drift (engine-list semantics, ordering, `supported` vs `enabled`). To make "one place"
as real as possible:

- The **Go** helper is the single source of truth; the **frontend** helper is the
  minimal acceptable duplication — a small pure function
  `(lastUsed, defaultEngine, engines[]) -> resolvedName` (the "enabled translate"
  predicate = `kind === 'translate' && enabled && supported`) whose *expected* outputs
  for a fixed engine list are captured in the t-red frontend test (d) by calling the
  **real** `EngineWrapper.PrimaryTranslateEngine()` over the same list. Any divergence
  between the two implementations fails the suite, which is the closest this architecture
  (two runtimes, one Wails boundary) allows to a single source of truth.
- The design must not claim the rule is implemented in one place; the honest statement
  is "one authoritative implementation (Go) + one mirror (frontend), divergence-checked
  by test (d)".
- "First" = first in `GetEngines()`/`GetAllEngines()` order, which is configstore
  `id` order (`configstore/query.sql:2` `ORDER BY id`; `engine_wrapper.go:151`).

Note: the translate window's list is `EngineListItem` from `GetEngines()` and has **no
`enabled` field** (`service/models.ts:48`); the `enabled`/`kind` data the frontend chain
needs comes from `GetAllEngines()` (`service/models.ts:7`, includes `enabled`). The
frontend helper therefore resolves against the `GetAllEngines()`-shaped list (what
`EnginesTab` already renders, `EnginesTab.svelte:188-190`); if the translate window keeps
only `EngineListItem`, it must additionally hold the `AllEngineItem` list for the `enabled`
check (the star state in settings uses the `GetAllEngines`-backed list, so both UIs
converge on the same data shape).

### 4. Translate window: read at window load; in-window switch persists as last-used

`TranslateWindow.svelte`:

- New state: `let lastUsedEngine = $state<string>('')`. Seed in `onMount` alongside the
  existing `Promise.all([loadDefaults(), loadEngines(), loadLanguages()])` (line 279):
  read `localStorage` key `kai:translate:lastEngine` via the existing `persisted()` store
  (`frontend/src/stores/persisted.ts:4` — real localStorage, `pinKey`-style key).
- Primary resolution on load and on every engine-list change:
  `resolvedPrimary = (lastUsed in enabled translate engines) ?? cfg.default_engine (if valid+enabled) ?? firstEnabledTranslateEngine`
  — this is the spec ordering *last-used ?? primary ?? first-enabled*. The engine-switch
  UI in the window (the per-engine selector in the result pane, the next issue) sets
  `lastUsedEngine` on change and writes it through the same `persisted()` store
  (localStorage write happens inside `persisted`'s subscriber, `persisted.ts:13`).
  Persisting last-used is **localStorage-only** (per spec "same pattern as persistLangs"
  applies to the read/persist mechanics; last-used is a per-window ephemeral preference,
  not a settings.json field — settings.json keeps the explicit user choice
  `default_engine`).
- On `EventEnginesChanged` (already subscribed, line 259) the window re-runs
  `loadEngines()` and re-resolves; if the last-used engine became disabled it silently
  falls through the chain to primary, then to first-enabled.
- The backend `PrimaryTranslateEngine()` is still the authoritative fallback for any
  non-frontend consumer (and for t-red tests (b)/(c)); the frontend chain must agree with
  it (covered by test (d)).

### 5. i18n keys (frontend, `frontend/src/i18n/en-US.ts` + `zh-CN.ts`)

All under `settings`:
- `settings.engineSetPrimary` — star tooltip/aria ("Set as primary engine" / 「設為主引擎」)
- `settings.enginePrimary` — optional group-section label or badge ("Primary" / 「主引擎」)
- `settings.enginePrimaryCleared` — info toast on un-set ("Primary engine cleared" / 「已取消主引擎」)
- `settings.engineNoEnabledEngine` — settings-side hint when no enabled translate engine exists
  ("Enable a translation engine first" / 「請先啟用一個翻譯引擎」)
Log keys under `log` (mirroring existing `log.persistLangPrefFailed` style):
- `log.setPrimaryFailed` — "Failed to set primary engine" / 「設為主引擎失敗」

(No new backend i18n keys are required: the resolution rule is structural, not user-facing text.)

### 6. SaveConfig roundtrip: why the star save cannot zero other settings

Verified, not assumed: `SaveConfig` (`config_wrapper.go:79-95`) does **not** replace the
settings object — it mutates the live `settingsSvc.Get()` pointer field-by-field
(`cur.Language = cfg.Language`, etc.) and then `Save()` → `writeConfig()` serializes that
same in-memory `*Settings` (`service.go:333-356`). A star save sends `{...cfg,
default_engine: name}`; for every field `SaveConfig` copies, the sent value **is** the
fresh `GetConfig()` value, so each self-assignment is a no-op and no field is zeroed.
`DefaultEngine` is simply added to the copy list.

Residual (pre-existing, out of scope, recorded for honesty): the `GetConfig()`-read →
`SaveConfig`-write sequence is not itself atomic — `Get()` returns the live pointer and the
copy window between it and `Save()` is unguarded (`settings.Service.mu` is only held
*inside* `Save()`, `service.go:359-363`), so a hot-reload or concurrent save in that window
could lose a field update. This exposure already exists for every settings field (language,
hotkeys, ...) via the same `persistLangs`/`applyAutoClipboard` read-modify-write pattern
(`TranslateWindow.svelte:321-328`, `52`); the star flow adds no new class of risk and the
design deliberately does not restructure settings concurrency here.

**CONFIRMED**: full-settings roundtrip preserves all untouched fields; the star flow does
not zero language/theme/hotkeys/TTS/etc.

## UI pattern verdict

**REUSE.** The set-as-primary affordance is a star **button** built from controls that
already exist in this file/approach set:

- The button class and active-state modifier are the existing pin button's:
  `u-icon-btn u-icon-btn--sm` + `class:u-icon-btn--active` — see
  `frontend/src/components/TranslateWindow.svelte:481-487` (pin/unpin icon button with
  active state) and `ScreenshotWindow.svelte:238`.
- Its placement *inside the `.u-list-item` row beside the name button and the `u-switch`*
  reuses the row layout at `EnginesTab.svelte:490-518`; the disabled/unsupported muted
  treatment reuses the `u-muted ... text-[10px]` span at `EnginesTab.svelte:498`.
- The immediate-persist interaction (click → SaveConfig read-modify-write →
  `emitEvent(EventEnginesChanged)` → error via `Dialogs.Error`/`parseErr`) is the existing
  `toggleEngine` pattern (`EnginesTab.svelte:318-346`) and `persistLangs`
  (`TranslateWindow.svelte:321-328`).

No new control type, widget or interaction model is introduced: the only delta is the ★
glyph and the `kind === 'translate'` rendering condition. A radio button would also have
been REUSE (per-row selection is the existing `selectedId` row model), but the star
matches the "set as primary" semantics of issue #8 better while reusing the same
  `u-icon-btn`/`--active` machinery.

**State reuse (confirmed)**: the star is driven entirely by state `EnginesTab` already
owns — the `engines` list is populated from `GetAllEngines()` (`EnginesTab.svelte:188-190`,
re-fetched by the existing `loadEngines()` on every mutation, lines 308/331/352/448) and the
list rows are the `group.items` of the existing `engineGroups` `$derived` (lines 33-48), so
`primaryEngine === e.value` plus `e.enabled`/`e.kind` need no new list source. The only new
state is the `primaryEngine` string, seeded at mount from `GetConfig().default_engine`
(the same `GetConfig` call pattern the tab's siblings use, `GeneralTab.svelte:27`).

## Tests (t-red; no mocks — real files/SQLite, real loopback HTTP, real jsdom)

Matches issue #8 "Tests" verbatim:

- (a) **Roundtrip**: configstore against `t.TempDir()` (real SQLite via
  `internal/configstore`); `settings.Service` in a temp data dir: set
  `cfg.DefaultEngine = "google"`, `Save()`, new service instance reads `"google"`.
- (b) **Invalid name → fallback**: `default_engine` set to a name not in the engine list
  (or kind ocr) → `PrimaryTranslateEngine()` returns the first enabled translate engine.
  Engine registry built against an `httptest.NewServer` loopback with the engine
  `Endpoint` pointed at it (real HTTP, no stubs) so "enabled" is a real configstore row.
- (c) **Primary later disabled → fallback re-resolves**: set primary, then flip the
  engine's `enabled` to false via the existing `ToggleEngineEnabled` path on the same
  configstore; `PrimaryTranslateEngine()` now returns the next enabled translate engine.
- (d) **Frontend resolution**: vitest in jsdom with real `localStorage` (the existing
  harness, `frontend/src/stores/persisted.test.ts` documents exactly this contract) and
  real timers; the pure resolution helper `(lastUsed, cfg.default_engine, engines[]) ->
  resolved value` is tested for: last-used wins; last-used disabled → primary; primary
  invalid/absent → first enabled; all empty → ''. **Divergence check**: the same engine
  lists are also fed to the Go `PrimaryTranslateEngine()` (real configstore, no mocks) and
  the test asserts the frontend mirror returns the same value the Go helper returns —
  this is the mechanism that keeps the duplicated rule honest (see 3).

No mocks anywhere: SQLite is real (`t.TempDir()`), HTTP is real loopback (`httptest`),
browser storage is real jsdom `localStorage`, timers are real.

## Out of scope (next issue)

- The result pane's engine selector UI (the *in-window switch* control) belongs to the
  result-pane issue; this issue only owns the `lastUsedEngine` state + persistence and the
  resolution rule it feeds.
- Multi-engine result ordering/priority beyond "first paint binding".

## Preflight result (recorded, not fixed — per bead instructions)

Run: `node .opencode/plugins/coding-pipeline/runner.ts preflight --issue 8 --rigor
second-opinion --ui-surface` (the `pipeline_preflight` tool itself is not in my tool
surface; the runner is the documented identical payload source, `runner.ts:13`).
Overall: `ok: false`. Failing rows (all environment/wiring, none product-code):

| check | status | detail |
| --- | --- | --- |
| role-models | fail | no model resolved for: orchestrator, tester, feature-implementor, architecture-reviewer, spec-auditor, designer, playwright-ui-walkthrough |
| toolchain:swiftc | fail | `swiftc --version` failed: not installed (Linux host, no Xcode CLT) |
| private-registry-auth | fail | `@performant-labs (https://npm.pkg.github.com): npm error code E403` |
| dual-review-interface | fail | rigor second-opinion but no reachable dual-review.sh (tried: dual-review.sh; workflow/dual-review.sh; docs/playbook/workflow/dual-review.sh; .agents/scripts/dual-review.sh) |
| gate-model-reachable | fail | no model resolved for the tester role, so there is no gate provider to probe |
| playwright-mcp | fail | uiSurface is true but no playwright MCP is configured in opencode.json |

Passing: pipeline-config, toolchain:go (1.27.1), worktree-present, handoffs-tracked,
orphaned-worktrees, issue-ready-to-brief. N/A: secret-scan, git-hooks-active.
Per the bead scope these rows are recorded and not acted on; the design stands.
