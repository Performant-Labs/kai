# Round-2 response — diff gate BLOCK remediation (run 0112-issue-112)

Answers dual-review(diff) round 1 (docs/handoffs/0112-issue-112/dual-review-diff.md). Every
claim below carries verbatim evidence with file:line, per the repo's verbatim-evidence rule.

## B-1 — AC7(b) mounted test: RESOLVED

T authored the missing mounted test in `frontend/src/components/translateWindowPinDefault.test.ts`
(commit f5f75e0): `#113 AC7(b): the explicit 🧷 choice wins over the setting toggle and the
Settings-close re-pin`. Sequence: mount with the setting ON and nothing stored (starts pinned
from the setting) → explicit 🧷 click to FALSE (v2 written "false") → setting toggled OFF,
broadcast → setting toggled back ON, broadcast → `kai:window:closing` 'settings'. Every leg
asserts `SetAlwaysOnTop` re-ran and stayed at the clicked value; storage ends exactly as the
click left it. The direction is deliberately discriminating (clicked false vs live setting
true at the second broadcast and the re-pin): an implementation that applies the setting or
the mount-time level instead of effectivePin's pinTouched branch visibly re-pins and fails.
Full frontend suite after the addition: `Test Files 82 passed (82)`, `Tests 890 passed (890)`.
NIT-1 honored in the same edit (the AC7(a) broadcast now fires payload-free).

## NV-1 — `get` import in TranslateWindow.svelte: VERIFIED (pre-existing)

`frontend/src/components/TranslateWindow.svelte:3`:
`import { get } from 'svelte/store';`
The import predates this diff — the file already used `get(sessionStore)` at `:256`
(`const restored = restoreSession(get(sessionStore));`). New use at `:58` (`get(pinnedStore)`)
compiles; vitest compiled the component in all 11 mounted tests.

## NV-2 — `app` provably non-nil before the hook/binding site: VERIFIED

`main.go:186` `var app *application.App` … `main.go:401` `app = application.New(appOpts)`.
The translate window itself is created THROUGH that value at `main.go:645`
(`translateWindow = app.Window.NewWithOptions(...)`), ~244 lines above the hook (main.go:689)
and the binding — a nil `app` could not have created the window the hook is registered on.
The old `if app != nil` guard was defensively redundant; its removal is behavior-neutral.

## NV-3 — cross-window event delivery (settings → open translate window): VERIFIED

Wails beta.24 `pkg/application/event_manager.go:31-35`:
`func (em *EventManager) Emit(name string, data ...any) bool { event := &CustomEvent{Name: name} ...`
`if err := em.app.customEventProcessor.Emit(event); err != nil {`
Emit goes through the APP-WIDE customEventProcessor (one per App, not per window) — a custom
event emitted from any window's frontend reaches every listener in the process. In-repo
production precedent, same shape, shipped since 0.5.x: `EventEnginesChanged` —
`frontend/src/components/settings/EnginesTab.svelte` emits after SaveConfig;
`TranslateWindow.svelte:883` (`onEvent(EventEnginesChanged, ...)`) receives it in the open
translate window. The new event uses the identical path.

## NV-4 — `NewWithOptions` returns `*WebviewWindow`; RegisterKeyBinding signature: VERIFIED

Wails beta.24 `pkg/application/window_manager.go:54`:
`func (wm *WindowManager) NewWithOptions(windowOptions WebviewWindowOptions) *WebviewWindow {`
`pkg/application/webview_window.go:1768`:
`func (w *WebviewWindow) RegisterKeyBinding(binding string, callback func(window Window)) *WebviewWindow {`
The comma-ok narrowing to `*application.WebviewWindow` therefore always succeeds (the value's
static type after assignment from NewWithOptions is the concrete pointer); "Cmd+W" parses
(`cmd`: CmdOrCtrlKey). Registration/firing on a real app remains a manual item (brief W-1).

## NV-5 — mount-time resolution always runs; creation-vs-mount corner: VERIFIED + disclosed

The mount IIFE's first block (post-diff, TranslateWindow.svelte ~:900-921) awaits `GetConfig()`
then calls `reapplyPin()` (resolvePinLevel → `pinnedStore.apply(effectivePin(...))` →
`Window.SetAlwaysOnTop($pinnedStore)`), before any other awaits — it runs on every mount, so
the frontend resolution always executes. The corner the reviewer names is real and disclosed:
with a stored v2 choice that CONTRADICTS the setting (e.g. stored false, setting on), Go
creates the window at the setting's level and the mount resolution corrects it to the stored
choice — a one-time level correction in the first instants of the first show after that
state arises. It cannot arise on a fresh install (nothing stored ⇒ creation and mount agree
on the setting); it needs an explicit 🧷 choice contradicting the setting. Accepted as
documented behavior; the alternative (creation reading the frontend store) would put
localStorage into Go's creation path.

## W-1 (broadcast on Cmd+W while hidden) — ACCEPTED, documented

HideTranslateWindow broadcasting even when the window is already hidden re-triggers the
frontend close cleanup (retained-session bookkeeping — idempotent by design: #81's retained
session clears nothing; autoRun.cancel/commitAutoNow are safe no-ops with nothing running).
Native Cmd+W cannot fire on a hidden window (no key view), so the reachable path is the
hotkey-triggered binding, and the cleanup is exactly what #57/#81/#109 want before a hide.
Recorded in handoff-F.md's disclosed nuances.

## W-2 (Cmd+W wiring test's 500-char window not anchored to the translate hook) — ACCEPTED

The grep window starts at `RegisterKeyBinding("Cmd+W"` — the FIRST occurrence in main.go, and
today the only one (the repo registers no other keybindings; A verified "kai registers no
keybindings today"). A second, earlier Cmd+W registration for another window would descope
the assertion — a fair brittleness note, accepted as-is: the anchored variant (search after
the WindowClosing hook index) is a test-shape change T may make in a future round; it does
not affect this diff's correctness.

## NIT-1 — RESOLVED with B-1 (payload-free broadcast; `fire`'s data param made optional).
