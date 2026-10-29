# Brief — issues #112 + #113 (run 0112-issue-112, one PR)

**Review rigor: second-opinion** (operator-set). **uiSurface: false** — Phase D (design) and
Phase U (ui-walkthrough) are **N/A, declared**: no new layout; the only UI-adjacent changes are
removing a title-bar button affordance and reusing existing controls/settings rows.
Outside gate: glm-5.3 via Z.ai (`./.env`, mirrored into this worktree). Round cap 2
(AGENTS.md); a BLOCK surviving round 2 stops the run for the operator.

## Objective

The translate window is the app's main surface; it must be un-closable (#112) and its
always-on-top start must follow the #69 setting (#113) — both with tested definitions at the Go
seams this repo already uses, and a CHANGELOG `[Unreleased]` line per user-visible change.

## Root causes (survey anchors in survey.md)

- **#112**: the WindowClosing cancel+hide hook (`main.go:689-703`) is the ONLY defense; the
  close button is enabled (`main.go:664`) and Cmd+W is unhandled. Destruction is permanent (no
  window recreation; `GetByName` → nil). The defect class: any close path that bypasses the hook
  bricks the app's window until relaunch, and nothing tests the rule.
- **#113**: `persisted`'s subscribe auto-writes the initial pin to localStorage
  (`persisted.ts:14-27`) and `initialPin` treats any stored value as an explicit choice, so the
  setting only ever governs a pristine profile (owner: stored `true` from the pre-#69 era).
  Go never applies the setting at creation; the settings toggle broadcasts nothing, so an open
  window never picks up a change.

## Acceptance criteria

**#112 — not closable, recoverable by construction:**
1. AC1: The translate window's close button is **hidden** (`CloseButtonState: ButtonHidden`) —
   no close affordance. Expected per platform (brief-gate W-3): macOS — the traffic-light close
   button is not shown; Windows — the title-bar close item is not shown; both are
   manual-verification items on a real app. (Issue leaves disabled-vs-hide open; hidden is the
   "cannot close" statement; the hook below keeps any residual path a hide, which the issue
   sanctions as "not a close". Settings/History/Screenshot windows unchanged.)
2. AC2: A named, unit-tested rule **in `internal/service/window_wrapper.go`** (the
   `ToggleTranslateWindow` shape; no new file — brief-gate NIT-2) implements the close policy:
   **broadcast `kevents.EventWindowClosing(model.WindowTranslate)` + Hide** (A Finding 2: the
   rule is broadcast+hide — a bare Hide would drop the frontend close cleanup #57/#81/#109 on the
   Cmd+W path). `main.go`'s hook composes `event.Cancel()` + the rule; the Cmd+W binding calls
   the rule directly. Tray-toggle `Hide` stays broadcast-free (existing, deliberate). RED first:
   the rule does not exist.
3. AC3: Cmd+W on the translate window deterministically **hides** it, never destroys, calling
   the same AC2 rule (broadcast + Hide). The **tested** claim is the rule (broadcast+hide
   counts, once per entry point) and the hook composition; the key-binding **registration and
   firing** is a manual-verification item on a real app (brief-gate W-1), wired in `main.go` via
   Wails `RegisterKeyBinding("Cmd+W", …)`. RED first: no tested rule; no binding.
4. AC4: The bring-back paths stay green: tray click toggle (`ToggleTranslateWindow` tests), the
   input hotkey show path, and the hook's own hide (all existing/extended tests pass).
5. Cmd+Q / quitting: untouched.

**#113 — the setting governs the level:**
6. AC5 (the test #113 names): a failing-then-green Go test over a pure creation-param function
   (`translateWindowStartsPinned(settingsService) bool`, the `translateWindowSize` shape — kept
   in `main_windowsize.go` per A Finding 5, with the file's description comment widened to
   "creation params (size, level)"): preference off → `false` (normal level), on → `true`,
   nil/unreadable → `false`. `main.go` feeds it into `AlwaysOnTop:` at window creation. RED
   first: no such function/option wiring.
7. AC6: the frontend pin store stops treating pollution as choice: the translate pin is stored
   under a **translate-specific versioned key** (e.g. `kai:translate:pinned:v2`; A Finding 3 —
   `pinKey()` itself is untouched so the screenshot window's stored pin is unaffected) and
   written **only on an explicit 🧷 toggle** (no subscribe auto-write; the mount-init resolution
   must not reach localStorage — A Finding 4's test: a mount with nothing stored leaves
   localStorage empty). `initialPin` semantics stay (a genuine stored choice still wins over
   the setting). Vitest RED first: with the old key present and the setting off, the resolved
   start pin must be `false`; store writes only on explicit set. One-time pin default reset is
   CHANGELOG'd.
8. AC7: toggling the setting broadcasts (the `EventEnginesChanged` settings→translate shape)
   and an open translate window applies it through the **effective-pin rule** (A Finding 1, the
   BLOCK amendment): **applied level = explicit 🧷 choice this mount (`pinTouched`) ? that
   choice : the current setting** — evaluated live at *every* application point: mount init,
   the setting-changed broadcast, and the Settings/History-close re-pin. The re-pin resolves
   through this decision; it must not apply the raw store. The pure decision is (A R2-1, the
   only completion consistent with AC6 + case (b)):
   `effectivePin(storedV2, pinTouched, thisMountChoice, liveSetting) = pinTouched ?
   thisMountChoice : initialPin(storedV2, liveSetting)` — one function at every application
   point; a stored v2 choice with no this-mount touch keeps winning even when Settings opens,
   the setting toggles, or Settings closes (the re-pin must not revert it). The pure decision
   lives in `frontend/src/utils/pinDefault.ts` (extended); Svelte wiring stays thin. Vitest
   matrix adds the
   interactions: (a) no explicit choice + setting toggled while open + Settings closed →
   level follows the new setting (the re-pin does not revert it); (b) explicit 🧷 choice +
   setting toggled + Settings closed → the explicit choice wins (#69's locked rule).
9. AC8: the existing re-pin on Settings/History close (`:852-861`) keeps working — rewritten to
   resolve through the effective-pin rule — and its test explicitly exercises the re-pin with
   the **versioned** key present (not only the old-key migration case; brief-gate W-2).

**Both:**
10. AC9: `CHANGELOG.md` `[Unreleased]`: two lines (not-closable window; on-top follows the
    setting + one-time pin default reset), `(#112)` / `(#113)` references.
11. AC10: full `test.unit.command` green at t-green; no `-skip` added; no test edits by F.

## Non-goals / guards

- No migration files (n/a — Go/Svelte, no DB), no changes to Settings/History/Screenshot
  windows, no new UI strings (no new controls), no `docs/` outside handoffs, CHANGELOG only per
  AC9.
- The screenshot window's pin semantics and the updater window are out of scope (survey
  disclosure).
- Scope stays inside: `main.go`, `internal/service/window_wrapper.go` (+ its tests),
  `main_windowsize.go`(+test), `frontend/src/components/TranslateWindow.svelte`,
  `frontend/src/components/settings/GeneralTab.svelte`, `frontend/src/stores/persisted.ts`
  (or a pin-specific sibling), `frontend/src/utils/pinDefault.ts`(+test), `frontend/src/utils/events.ts`,
  `CHANGELOG.md`, `docs/handoffs/0112-issue-112/**`. Other parallel runs own other files — do
  not touch anything else.
- What cannot be verified here (say plainly in the PR): real-app behaviour on a Mac (red X with
  a hidden button, Cmd+W key binding firing, level going behind other apps) needs the signed app
  on a real desktop — listed as manual verification, not claimed as tested.

## Handoffs

- Run dir: `docs/handoffs/0112-issue-112/` (this brief, survey.md, decisions.md, per-phase
  handoffs). Branch `issue-0112-issue-112`; PR to `Performant-Labs/kai` `master`, milestone
  **0.6.1**, closing wording for #112 and #113.
- Phase order and gates: plugin-owned (second-opinion brief + diff gates via
  `pipeline_dual_review`, round cap 2).
