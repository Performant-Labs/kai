# Changelog

Changes in this fork (`Performant-Labs/kai-private`) since it diverged from upstream `dtapps/kai`.
Format and release steps: [`docs/releasing.md`](docs/releasing.md). Add a line under
`[Unreleased]` when a change merges; a release moves that section under a version heading.

## [Unreleased]

### Enhancements
- Long source text is split per engine's input budget and the translation reassembled (#84).
- Per-engine input budgets, plus an opt-in Apple size probe; Apple's input limit measured at 18,200 characters (#83, #119).
- The 20-second wait on Apple translation is gone and Apple translation can be cancelled (#111).
- No fixed translation time limit: progress is shown and the user can cancel (#109).
- Undo and redo for the source text (typing, Clear, swap, fills) (#118).
- The result pane is flat: real text, paragraph breaks and spacing kept, selection and caret kept (#95, #144).
- When translation fails, Kai says why (#96).
- When source and target language are the same, the source text is shown with a short note (#80).
- The source language is auto-corrected when a pinned language does not match the text; the source dropdown never changes and a note under the result names the detected language. Its Auto entry now reads "Detected" (#161).
- Language dropdowns lead with English, Spanish (Mexico), Portuguese (Portugal); Cmd+Enter translates; source and result pane headers match in height; tooltips explain the pin and clipboard buttons (#165).
- Kai has a Dock icon and appears in Cmd+Tab (#165).
- English is the default interface language; "auto" follows the system (#77).

### Bug Fixes
- Launch no longer aborts because of semicolons in SQL comments (#64).
- Windows are opted out of macOS window-state restoration so a previously open window should not reappear at launch (#163). Not yet confirmed by a kill-and-relaunch test.
- Launch no longer crashes about a second in on macOS (regression from #163, fixed in #167).
- A restored translate window no longer shows a failure that belongs to a previous run's request (#116).

### Known Issues
- Alt+A can miss or mistranslate selected text in Chrome (for example a Google Sheets cell) and in WhatsApp: the simulated copy does not always reach the app. Fix in progress in #175.
- Apple's on-device engine can add blank lines between translated lines of multi-line input. Believed to be Apple's own behavior; not confirmed against another engine (#173).
- The in-app updater is hardcoded to upstream `dtapps/kai` and can offer an upstream build over this one. Decline the update prompt. See `docs/releasing.md`.
- Every new build is a new identity to macOS, so the Accessibility grant must be removed and added again after installing one.
