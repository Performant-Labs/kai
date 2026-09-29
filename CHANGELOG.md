# Changelog

Changes in this fork (`Performant-Labs/kai-private`) since it diverged from upstream `dtapps/kai`.
Format and release steps: [`docs/releasing.md`](docs/releasing.md). Add a line under
`[Unreleased]` when a change merges; a release moves that section under a version heading.
`scripts/changelog-check.sh` lists merged PRs this section does not cover.

## [Unreleased]

### Enhancements
- **Visible behaviour change, on by default:** all text in Kai is now 20% larger than before (#195). New in Settings > General > "Text size": smaller and larger buttons step through 90%, 105%, 120%, 135%, 150% and 165% of the old size (15 percentage points per step), and a reset returns to 120%, the new default. It applies to every window immediately, without a restart, and is saved with your other settings; a settings file from before this change reads as 120%, so everyone sees the larger text after updating until they pick another size. Only text grows: spacing, icons and pane proportions stay, and the translate and screenshot windows open 20% wider so the toolbars still fit (a window size you saved by resizing is kept, unless it is narrower than the new minimum, when it starts at the default again). The settings window stays fixed at 1280x800. Native macOS menus and system dialogs do not follow the setting.
- **Visible behaviour change, on by default:** when text arrives in a different language than the pinned source dropdown shows, Kai now switches the source dropdown to that language, makes the old source the target and translates, with no click. It applies to the hotkey, tray and auto-clipboard fill, a paste, and Translate on typed text, and a note under the result says the languages were switched; the swap button undoes it (#200). This reverses #161's "the dropdown never changes" for a pinned source; an Auto source still never switches and keeps its "auto-detected" note. Only text of 20 or more characters and a confident detection switch anything, regional variants (Spanish (Mexico)) are kept, and a switch is never saved as your default languages. Language detection is local on macOS (Apple's NaturalLanguage). Turn it off in Settings > General > "Auto-switch source language".
- New, off by default (macOS only): **translate on double Cmd+C** (#199). Turn it on in Settings > General > "Translate on double Cmd+C", then press Cmd+C twice within half a second in any app: Kai opens with the copied text filled in and translates it, through the same path as the hotkey and auto-clipboard fills (so #200's source switch applies). It needs the Input Monitoring permission, which Kai requests when you switch the option on; if it is missing, Kai says to enable it under System Settings > Privacy & Security > Input Monitoring. The listener only listens (your Cmd+C is never delayed or changed), keeps no other key, fires only when the clipboard really changed (a Cmd+C with nothing selected does nothing), fires once for three or more quick presses, ignores Kai's own simulated copy, and never translates a copy made in a password manager or keychain app (1Password, Bitwarden, Dashlane, LastPass, Keychain Access, Passwords) or one marked concealed/transient. The 500 ms window is fixed for now; an adjustable window and Windows support are follow-ups.
- The in-app updater no longer looks at upstream Kai's releases, so it can no longer offer an upstream build over this one. It checks this fork instead (#178). This fork's repository is private, so for now the check finds nothing and stays silent: updates are not available until releases are published somewhere public, and each release is installed by hand.
- The China mirror (cnb.cool) update source is removed. A saved "cnb" update-source setting is reset to the default on the next launch (#178).

## [0.1.0] - 2026-09-29
<!-- changelog-skip: PRs deliberately not listed (CI, test-harness and i18n-key-check plumbing, and a groundwork PR replaced by #147): #4 #21 #27 #37 #71 -->

### Enhancements
- Two-pane, side-by-side translate window (#10).
- The translate window stays above other apps until it is hidden (#39).
- Each engine's failure reason is shown (#42), and a failed translation says why (#96).
- Spanish (Mexico), Portuguese (Brazil) and Portuguese (Portugal) can be chosen as source and target, with each engine's support taken from the backend (#43, #52). Choosing a dialect once makes later auto-detected Spanish or Portuguese use it (#53).
- Swap moves the result into the source and reverse-translates, dialect-aware (#13).
- New installs translate into English by default, and a saved target that cannot be used falls back to it (#44); a target you saved is never rewritten. When source and target language match, the source text is shown with a short note (#80).
- Clicking the tray icon opens the translate window, and a gear icon there opens Settings (#69).
- The translate window keeps the last source and target text and language choices across close and reopen (#81).
- English is the default interface language; "auto" follows the system (#77).
- Long source text is split per engine's input budget and the translation reassembled (#84).
- Per-engine input budgets, plus an opt-in Apple size probe; Apple's input limit measured at 18,200 characters (#83, #119).
- Apple translation no longer waits a fixed 20 seconds and can be cancelled (#111); no engine has a fixed time limit, progress is shown and any request can be cancelled (#109).
- Apple's behavior at large sizes is measured and recorded: output quality by input size and the wait after a cancel (#152, #153, #155, #156, #157, #158; see `docs/engine-limits.md` and `docs/quality-limits.md`).
- Undo and redo for the source text (typing, Clear, swap, fills) (#118).
- The source and result panes show real text: paragraph breaks and spacing kept, selection and caret kept (#95, #144).
- Auto source detection shows what was detected (#11). A pinned source language that does not match the text is corrected from a single call; the dropdown never changes, a note under the result names the detected language, and its Auto entry reads "Detected" (#161). Since #200 a pinned source does change: see Unreleased.
- Language dropdowns lead with English, Spanish (Mexico), Portuguese (Portugal); Cmd+Enter translates; the source and result pane headers match in height; tooltips explain the pin and clipboard buttons (#165).
- Kai has a Dock icon and appears in Cmd+Tab (#165).
- The Translate button shows a Cmd+Enter hint; the translate window's size is remembered across launches; tooltips describe what each button does; the Apple translation session is prepared at launch and reused (#173).
- The settings window is a fixed size (#175).

### Bug Fixes
- Launch no longer aborts because of semicolons in SQL comments (#64).
- An idle translate window no longer shows "Translation failed" (#81), and a restored window no longer shows a failure that belongs to a previous run's request (#116).
- Windows are opted out of macOS window-state restoration so a previously open window should not reappear at launch (#163). Not yet confirmed by a kill-and-relaunch test.
- Launch no longer crashes about a second in on macOS (a regression from #163, fixed in #167).
- Alt+A waits for the clipboard to change instead of reading it once 120 ms after the copy, an overlapping hotkey press no longer races the first on the clipboard, and a copy that fails shows a message instead of translating stale text (#173, #175).

### Known Issues
- Alt+A can still miss selected text in Chrome (for example a Google Sheets cell). A probe found the simulated Cmd+C fails against a Chrome text field at every key delay tried, so the fixes above may not help there. WhatsApp got the race fix but was not tested inside WhatsApp itself (#175).
- After shrinking the translate window, quitting and relaunching, one report said the window did not appear (possibly behind other windows). It was not reproduced (#175).
- Apple's on-device engine can add blank lines between translated lines of multi-line input. Believed to be Apple's own behavior; not confirmed against another engine (#173).
- The Baidu engine keeps only the first entry of Baidu's response (`internal/engine/baidu.go`), so if Baidu returns one entry per line, multi-line text loses every line after the first. Baidu's real response was not tested (#146).
- The Settings window was reported to disappear when Kai loses focus. That was filed against the old menu-bar-only mode; the Dock icon change (#165) may have fixed it, and it has not been re-tested (#154).
- The in-app updater is hardcoded to upstream `dtapps/kai` and can offer an upstream build over this one. Decline the update prompt (#178). Fixed after 0.1.0: see [Unreleased].
- Every new build is a new identity to macOS, so the Accessibility grant must be removed and added again after installing one.
