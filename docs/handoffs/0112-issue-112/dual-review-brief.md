## Brief Review (Round 1)

### BLOCK findings
None.

### Needs-verification findings
**[NV-1]** The claim that `persisted.ts:14-27` subscribe auto-writes the initial pin to localStorage and that `initialPin` treats any stored value as explicit choice — the excerpt set does not include `persisted.ts`; a verbatim excerpt of `frontend/src/stores/persisted.ts` lines 1–60 (plus `pinDefault.ts`) would settle it.
**[NV-2]** The claim that `translateWindowSize` in `main_windowsize.go` is a pure creation-param function of settingsService of the shape AC5 assumes — an excerpt of `main_windowsize.go` with the function and its test would settle it.
**[NV-3]** The claim that `ToggleTranslateWindow` and `EventAutoClipboardChanged` exist with the cited shapes in `internal/service` / `kevents` — excerpts of `window_wrapper.go` (the toggle tests) and the events declarations would settle it.
**[NV-4]** The claim that Wails supports a Cmd+W key binding whose action can call the hide path (AC3) — the Wails v3 API documentation/source for key bindings (e.g. `application.KeyBinding` / accelerator registration) would settle it.
**[NV-5]** The claim that `CloseButtonState: ButtonHidden` actually removes the red X affordance on macOS (vs. leaving an inert traffic light) — Wails source or documentation for `ButtonHidden` on the Mac title bar would settle it; the brief itself defers real-Mac behavior to manual verification.
**[NV-6]** The claim that the re-pin on Settings/History close exists at `main.go:852-861` — that range is not in the excerpt (excerpt ends at 715); an excerpt of `main.go:840-870` would settle it.
**[NV-7]** The claim that window destruction is permanent (no recreation, `GetByName` → nil) — not shown in any excerpt; Wails documentation/source for window recreation or a repo citation would settle it (it underpins the "by construction" framing of #112).

### WARN findings
**[W-1]** AC3 says Cmd+W "deterministically hides" while the Non-goals section says Cmd+W firing "needs the signed app on a real desktop — listed as manual verification." The AC as written is only partially testable in-unit (the action can be tested; the binding firing cannot). Recommendation: reword AC3 to scope the tested claim to the action, and keep binding registration as a manual-verification item, mirroring the brief's own disclosure.
**[W-2]** AC6 changes shared pin-storage semantics while AC8 requires the Settings/History re-pin path keep working "with the new store" — the interaction between the versioned-key store and the `:852-861` re-pin (which is not shown) is unverified. Recommendation: ensure AC8's test explicitly exercises the re-pin with the versioned key present, not just the old-key case.
**[W-3]** AC1 fixes "hidden" over "disabled" citing the issue's openness, but hides a fallback concern: if `ButtonHidden` behaves differently across macOS/Windows (both platforms are configured in the shown excerpt, lines 657-664), the criterion is platform-untestable here. Recommendation: state the per-platform expected behavior explicitly in the AC or PR description.

### NIT findings
**[NIT-1]** Excerpt line 664 (`CloseButtonState: application.ButtonEnabled`) and the hook at 689-703 match the brief's root-cause description — consistent; no action.
**[NIT-2]** AC2 names the rule "the `ToggleTranslateWindow` shape" but places it in `internal/service`; scope list names only `window_wrapper.go` (+tests) in that package — confirm the new rule file is intended under `internal/service/window_wrapper.go` to avoid an out-of-scope file write.

### Verdict
PASS — no BLOCK findings; implementation may proceed.