# Survey — issues #112 + #113 (run 0112-issue-112)

Scope: the translate window's close behaviour (#112) and its always-on-top start/runtime
behaviour vs the #69 setting (#113). Both live in window creation (`main.go`) + window rules
(`internal/service/window_wrapper.go`) + the translate window frontend
(`frontend/src/components/TranslateWindow.svelte`). No new layout → uiSurface false.

## What the issues say (source of truth)

- **#112**: the main (translate) window must not be closable. Red button and Cmd+W must not
  close it; Hide / tray / toggle shortcut must keep working so the window can always be brought
  back. To settle: red button disabled vs hide (hide is explicitly "not a close"); Cmd+Q stays
  as-is (quitting unchanged).
- **#113**: with Settings > General "Keep the translate window on top of other apps" (#69
  setting, off by default) OFF, the window still stays on top. Wants: preference off → new
  window starts at a normal level and goes behind other apps, unless per-window pinned; suspects
  creation-time pinning; needs a failing test creating the window with the preference off and
  asserting the level; also check runtime pickup of a changed preference by an open window.

## The closing machinery today (verbatim anchors)

- `main.go:689-703` — the translate window's only close defense is the WindowClosing hook:
  `sed -n '689,703p' main.go` → `_ = translateWindow.RegisterHook(events.Common.WindowClosing,
  func(event *application.WindowEvent) { event.Cancel(); … Emit(kevents.EventWindowClosing,
  model.WindowTranslate) … translateWindow.Hide() })`. Red X today = cancel + broadcast + Hide.
- `main.go:661-664` — the affordance stays on: `MinimiseButtonState: ButtonEnabled,
  MaximiseButtonState: ButtonHidden, CloseButtonState: ButtonEnabled`.
- Wails v3.0.0-beta.24 (go.mod:20; 0.6.0 shipped the same) darwin close chain:
  `webview_window_darwin.m:478-494` `windowShouldClose:` → `processWindowEvent(…,
  EventWindowShouldClose)` and returns NO; `pkg/events/defaults.go:43`
  `Mac.WindowShouldClose: Common.WindowClosing`; `webview_window.go:366-371` wails' own
  WindowClosing LISTENER destroys the window (`markAsDestroyed` + `impl.close`); hooks run before
  listeners and a `Cancel()` (`webview_window.go:992-1002`) skips them. So the hook is a
  correct-but-sole event-path defense.
- **Gap (the #112 defect class):** destruction is permanent — kai never recreates a window after
  startup (`window_wrapper.go:34-39` `GetByName` fails → `log.window_handle_failed` → nil), so
  ANY close path that bypasses the hook (a menu-bar Close/Cmd+W item, a programmatic `Close()`,
  a Wails behaviour change) leaves the app windowless until relaunch. Nothing pins this rule in
  a test; `CloseButtonState` invites the close; Cmd+W is unhandled and undefined ("if the window
  handles it").
- Hide paths that must keep working: tray click `main.go:1003-1005`
  (`service.ToggleTranslateWindow(translateWindow, windowSvc.ShowTranslateWindow)`), the input
  hotkey (`hm.TriggerInput` → EventInputFill/show), and the red-X hook itself. Cmd+Q / app quit
  is out of scope (issue: "quitting stays as is").

## The always-on-top machinery today (verbatim anchors)

- Go creation (`main.go:645-665`): NO `AlwaysOnTop` in the translate window's options — the
  level is whatever the frontend applies after mount. Nothing consults the setting at creation.
  The setting: `internal/settings/service.go:57-65` (`TranslateWindowOnTop`, default off,
  `readTranslateWindowOnTop` :329), hot reload :534, save :580.
- Frontend mount (`TranslateWindow.svelte:900-921`): `pinnedStore.set(initialPin(storedPinRaw,
  cfg?.translate_window_on_top === true))` then `Window.SetAlwaysOnTop($pinnedStore)`.
  `initialPin` (`frontend/src/utils/pinDefault.ts`): stored value wins, always; only absence
  falls to the setting.
- **Root cause (the #113 defect):** `frontend/src/stores/persisted.ts:14-27` — `store.subscribe`
  auto-writes the initial value to localStorage on creation, and `initialPin` treats ANY stored
  value as an explicit choice. So every install accumulates a non-choice stored pin: the owner's
  `true` (pre-#69 era: #39's forced pin + the auto-write), fresh installs' `false` (auto-written
  at first mount, later masking a setting=ON). The setting therefore only ever applies to a
  pristine profile — exactly "with the switch off, the window still stays on top".
- Runtime pickup: `GeneralTab.svelte:128-139` saves `translate_window_on_top` but broadcasts
  nothing; the translate window reads it once per mount (`:903` "read once per mount … not a
  live toggle"). An open window never learns a changed setting — #113's "also check".
- Re-pin on Settings/History close: `TranslateWindow.svelte:852-861` re-applies
  `$pinnedStore` on `EventWindowClosing(settings|history)`; `service.LowerForSettings`
  (`window_wrapper.go:148-153`) lowers a visible pinned window first.
- Sibling disclosure (NOT in scope): the screenshot window's pin deliberately never unpins
  (`ScreenshotWindow.svelte:66-72` "never call SetAlwaysOnTop(false) … level, set at
  [creation]"; its main.go comment `:730` mentions AlwaysOnTop in init prose only). Different
  window class (modal overlay); its comments about "main.go's AlwaysOnTop:true" are stale but
  its behaviour is intentional. Updater window
  (`pkg/wails-updater-providers/updater_window.go:144 AlwaysOnTop: true`) is separate too. No
  change to either in this story.

## Reuse & Analogous-Feature map (extend, not new)

| Need | Closest existing seam | Recommendation |
|---|---|---|
| Testable close rule for the translate window | `service.ToggleTranslateWindow` / `LowerForSettings` / `showHistory` — package-level funcs over small window interfaces in `window_wrapper.go`, faked in `window_toggle_test.go` / `window_history_test.go` | **Extend**: extract the hook body into a named rule (cancel + broadcast + hide) beside them; test with the existing fake-window pattern |
| No close affordance | `CloseButtonState` on every window in `main.go` (Settings :546, History :607, Translate :664, Screenshot :727) | **Extend**: translate window's close button → hidden (or disabled); Settings/History keep hide-on-close (auxiliary views; their hook already cancels+hides) |
| Cmd+W deterministic | Wails `KeyBindings` / `RegisterKeyBinding` (beta.24 `webview_window.go:368-382`) | **New but tiny**: bind Cmd+W → Hide on the translate window; the callback body is the tested `hide` rule shape |
| Creation-time level from the setting | `translateWindowSize(settingsService, …)` (`main_windowsize.go:33-46`) — a settings-derived creation param with its own test file | **Extend**: `translateWindowStartsPinned(settingsService) bool` sibling; feed `AlwaysOnTop:` in `main.go` options. THE failing test #113 names |
| Pin store that cannot be polluted | `persisted` (auto-writes on subscribe) vs a pin-specific store that writes only on explicit set | **Extend narrowly**: keep `persisted` for other stores; the pin store becomes explicit-write only + a one-time key version bump to drop polluted history |
| Settings → translate broadcast | `EventAutoClipboardChanged` (`events.ts:59`; GeneralTab emits after SaveConfig, translate listens) | **Extend**: same shape for the on-top setting → translate applies it when no explicit 🧷 choice was made this mount (`pinTouched` guard already exists :27) |

## Test-command reality

`test.unit.command` (worktree config): swiftbridge build → `go test ./internal/... ./pkg/...` →
`pnpm --dir frontend test` → the two release harnesses. Frontend needs
`NODE_OPTIONS=--no-experimental-webstorage` (already inside `frontend/package.json`'s test
script since #99). Go RED/GREEN will live in `internal/service` (+ `main_windowsize_test.go`
shape for the creation param); frontend RED/GREEN in `pinDefault`-style vitest files.
