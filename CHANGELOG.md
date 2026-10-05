# Changelog

Changes in this fork (`Performant-Labs/kai-private`) since it diverged from upstream `dtapps/kai`.
Format and release steps: [`docs/releasing.md`](docs/releasing.md). Add a line under
`[Unreleased]` when a change merges; a release moves that section under a version heading.
`scripts/changelog-check.sh` lists merged PRs this section does not cover.

## [Unreleased]

### Enhancements
- Both `[source switch]` log lines (the decision and "skipped by the window") now carry `version=` and `commit=`, so a pasted `kai.log` says which build produced a line, also when the startup line is in another day's file. This is for diagnosing the intermittent Alt-A miss (#16).

### Bug Fixes
- The swap button in the translate window now always swaps for real. After an automatic source switch (a Spanish text moved the source to Spanish and the target to English), it used to undo the switch instead: the dropdowns changed but the source pane kept its Spanish text and the result came back in Spanish. Now the source pane shows the translation, the old text's language becomes the target, and each pane matches its dropdown. The note of the automatic switch no longer says "Swap to undo" (#39).

## [0.4.0] - 2026-10-02
<!-- changelog-skip: release PR plumbing, no user-facing change: #33 -->

### Enhancements
- Releases are now signed with a stable self-signed certificate ("Kai Release") instead of ad-hoc, so macOS identifies the app by that certificate and not by the hash of each build (#36). This does not make the app trusted by macOS: Gatekeeper still blocks a browser download (see Known Issues).

### Breaking Changes
- **The app's identifier changed** from `net.dtapp.kai` to `com.performantlabs.kai` (the development app, Kai-dev, from `net.dtapp.kai.dev` to `com.performantlabs.kai.dev`), under Performant Labs' own namespace instead of upstream's (#27). To macOS this is a new app. Grant Accessibility (Device Control and Data Access), Screen Recording and Input Monitoring again in Settings > Shortcuts; the old "Kai" rows stay in Privacy & Security, so remove them. Quit an older Kai before opening this one: the two do not share the single-instance lock, so both would run and fight over the tray and the hotkeys. Kai's own settings and history (`~/.kai`) carry over; the few interface preferences the web view remembers (the last Settings tab, a window's pin) start fresh. Details: `docs/dev-signing.md`.

### Bug Fixes
- The log line Kai writes for each decision of the automatic source switch now shows its words ("[source switch] decision", or "skipped by the window") instead of the raw message key, which 0.3.0 printed because the texts were missing from the language sources. Search the log for `[source switch]` (#29, #30). Nothing else changed since 0.3.0.

### Known Issues
- Alt-A sometimes captures the right Mexican Spanish text but does not switch the language drop-downs (at least 3 of 20 translations for one user). The cause is not found yet; Kai logs every decision, so the next miss can be diagnosed ([#16](https://github.com/Performant-Labs/kai/issues/16)).
- Kai's log records the text you select or copy on the hotkey paths (Alt+A and the copy key) at info level, so it can contain private text; do not paste a log into an issue without checking it ([#11](https://github.com/Performant-Labs/kai/issues/11)).
- Screen Recording: macOS may not list Kai in Privacy & Security > Screen & System Audio Recording when you click Grant access (it did not for the development build; for this release it is unverified). If Kai is not listed, click + in that list, add Kai, switch it on, then restart Kai.
- The title bar of the Settings and translate windows is white while the content is dark in the dark theme ([#8](https://github.com/Performant-Labs/kai/issues/8)).
- The in-app updater has not yet been seen to find a release on a real install, so do not count on it, and this release changes the app's identifier: install 0.4.0 by hand (the disk image, the zip, or `scripts/install-release.sh`). Kai 0.2.0 looks in the archived private repository and cannot offer any update.
- Permissions must be granted again once after installing 0.4.0 (the identifier changed). Releases are now signed with a stable certificate, so later updates should keep them, but that is not yet verified on other Macs ([#36](https://github.com/Performant-Labs/kai/issues/36)); if Alt+A stops copying after an update, remove Kai from the list in Privacy & Security and add it again.
- The certificate is self-signed, so macOS still blocks a copy downloaded with a browser as coming from an unidentified developer: install with `gh release download` and `scripts/install-release.sh`, or run `xattr -dr com.apple.quarantine /Applications/Kai.app`.

## [0.3.0] - 2026-10-01

### Enhancements
- **Visible behaviour change, on by default:** Kai now shows the translate window every time you open it from the Dock or Finder, not only on a later click of the tray or Dock icon (#17). The very first launch on a fresh data folder centres the window. Launching at login stays quiet, with no window, and opening Kai a second time while it runs still brings the window forward. Where macOS cannot say how Kai was launched, the window is shown.
- Settings > Shortcuts re-checks the macOS permissions every 3 seconds while the Settings window is showing, so switching Device Control and Data Access, Screen Recording or Input Monitoring on or off in System Settings changes its row within about 3 seconds, without reopening the page (#14). Checking pauses while Settings is hidden or closed. Input Monitoring now has its own row (it is needed for translate on double Cmd+C), and the note under that switch follows the live permission. A Screen Recording grant still needs Kai to be restarted before Kai sees it (macOS applies it to a running app only after a restart).
- Every decision of the automatic source switch now writes one line to Kai's log (the languages, the confidence, the length, who answered, and why it did or did not switch, never the text), so a missed switch can be diagnosed (#16).
- The in-app updater, the installer and the release scripts now point at the public repository Performant-Labs/kai instead of the archived private one, and the release preflight still refuses upstream dtapps/kai (#196, #26).

### Bug Fixes
- The white flash when a window opens in the dark theme is gone (#15, #22). The window's native colour is chosen from the theme before it draws, each page applies the last saved theme before its first paint, and each window's web view no longer draws its own white background when a hidden window is shown again (Settings flashed white on every open, and the translate window at launch). The colours follow the theme when it changes (System follows the Mac's appearance). Checked in the real app.
- The permission rows in Settings > Shortcuts are clearer (#23): the first row is named Device Control and Data Access, as macOS 27 calls it; a Grant access button shows only while that permission is missing (Input Monitoring is already covered by Device Control and Data Access on macOS 27, so its row shows Granted); and a missing Screen Recording permission says to click + in Screen & System Audio Recording to add Kai, then restart Kai.
- A selection that is only a link no longer switches the language drop-downs because of Spanish words in its path (#16).

### Known Issues
- Alt-A sometimes captures the right Mexican Spanish text but does not switch the language drop-downs (at least 3 of 20 translations for one user). The cause is not found yet; this release logs every decision so the next miss can be diagnosed ([#16](https://github.com/Performant-Labs/kai/issues/16)).
- Kai's log records the text you select or copy on the hotkey paths (Alt+A and the copy key) at info level, so it can contain private text; do not paste a log into an issue without checking it ([#11](https://github.com/Performant-Labs/kai/issues/11)).
- Screen Recording: macOS may not list Kai in Privacy & Security > Screen & System Audio Recording when you click Grant access (it did not for the development build; for this release it is unverified). If Kai is not listed, click + in that list, add Kai, switch it on, then restart Kai.
- The title bar of the Settings and translate windows is white while the content is dark in the dark theme ([#8](https://github.com/Performant-Labs/kai/issues/8)).
- Kai 0.2.0 cannot offer this update: it looks for updates in the archived private repository, which it cannot read. Install 0.3.0 by hand (the disk image, the zip, or `scripts/install-release.sh`); from this release on Kai looks in the public repository.
- Every new build is a new identity to macOS, so the Accessibility grant must be removed and added again after installing one.

## [0.2.0] - 2026-09-29

### Enhancements
- **Visible behaviour change, on by default:** all text in Kai is now 20% larger than before (#195, #205). New in Settings > General > "Text size": smaller and larger buttons step through 80%, 100%, 120%, 140%, 160% and 180% of the old size (20 percentage points per step), and a reset returns to 120%, the new default. It applies to every window immediately, without a restart, and is saved with your other settings; a settings file from before this change reads as 120%, so everyone sees the larger text after updating until they pick another size. Only text grows: spacing, icons and pane proportions stay, and the translate and screenshot windows open 20% wider so the toolbars still fit (a window size you saved by resizing is kept, unless it is narrower than the new minimum, when it starts at the default again). The settings window stays fixed at 1280x800. Native macOS menus and system dialogs do not follow the setting.
- **Visible behaviour change, on by default:** when text arrives in a different language than the pinned source dropdown shows, Kai now switches the source dropdown to that language, makes the old source the target and translates, with no click. It applies to the hotkey, tray and auto-clipboard fill, a paste, and Translate on typed text, and a note under the result says the languages were switched; the swap button undoes it (#200, #201). This reverses #161's "the dropdown never changes" for a pinned source; an Auto source still never switches and keeps its "auto-detected" note. Only text of 20 or more characters and a confident detection switch anything, regional variants (Spanish (Mexico)) are kept, and a switch is never saved as your default languages. Language detection is local on macOS (Apple's NaturalLanguage). Turn it off in Settings > General > "Auto-switch source language".
- New, off by default (macOS only): **translate on double Cmd+C** (#199, #202; moved to Settings > Shortcuts in #213, see ADR-0003 / #122). Turn it on in Settings > Shortcuts > "Translate on double Cmd+C", then press Cmd+C twice within half a second in any app: Kai opens with the copied text filled in and translates it, through the same path as the hotkey and auto-clipboard fills (so #200's source switch applies). It needs the Input Monitoring permission, which Kai requests when you switch the option on; if it is missing, Kai says to enable it under System Settings > Privacy & Security > Input Monitoring. The listener only listens (your Cmd+C is never delayed or changed), keeps no other key, fires only when the clipboard really changed (a Cmd+C with nothing selected does nothing), fires once for three or more quick presses, ignores Kai's own simulated copy, and never translates a copy made in a password manager or keychain app (1Password, Bitwarden, Dashlane, LastPass, Keychain Access, Passwords) or one marked concealed/transient. The 500 ms window is fixed for now; an adjustable window and Windows support are follow-ups.
- New, off by default (macOS only, needs Apple Intelligence): **correct grammar and wording** (#208, #210). Tick "Correct grammar and wording" in the translate window's toolbar and, when text arrives with a pinned source language, Apple's on-device model first fixes its grammar, agreement, tense and word choice, keeping the regional variant (Mexican Spanish stays Mexican Spanish), and replaces mixed-in foreign words (Spanish with English in it becomes Spanish). The corrected text is what gets translated; your original stays in the source pane, the result pane lists what changed, and one button translates the original instead. Nothing leaves the Mac, and nothing is remembered as a default. The checkbox is disabled, with the reason as its tooltip, when the model cannot run (Apple Intelligence off, model not ready, unsupported Mac or OS). It does nothing for an Auto source or for text under 8 characters. **Unverified:** how good the model's Mexican Spanish is has not been judged yet; it is the first thing the owner's real use will show (phase 1 of #208).
- When the macOS Accessibility permission is missing (shown as "Device Control and Data Access" on macOS 27), Alt+A and the tray input entry now say so instead of failing like "nothing selected" (#194, #211, #215, #216, #218). Before sending the simulated Cmd+C Kai checks the permission; if it is missing it sends no key and leaves your clipboard untouched, shows the translate window, and shows one short message: "Kai can't copy your selection. Turn on Kai in Privacy & Security > Device Control and Data Access." The message is shown at most once every 3 minutes, so repeated presses do not repeat it; in between the window still opens. Settings > Shortcuts shows a one-line note under the Accessibility row when it is not granted. If the permission is granted but nothing was copied, you still get the usual "couldn't capture the selection" message. Kai never asks for the permission by itself: the "Grant access" button does that. Confirmed on a real build: with Kai removed from the list, Alt+A showed the message, and after granting, Kai read the permission as granted.
- The in-app updater no longer looks at upstream Kai's releases, so it can no longer offer an upstream build over this one. It checks this fork instead (#178, #193). This fork's repository is private, so for now the check finds nothing and stays silent: updates are not available until releases are published somewhere public, and each release is installed by hand.
- The China mirror (cnb.cool) update source is removed. A saved "cnb" update-source setting is reset to the default on the next launch (#178).

- Releases now come with a disk image (.dmg) next to the zip and a `SHA256SUMS` file covering them, and `scripts/install-release.sh` installs a release without the macOS "unidentified developer" quarantine block (#190, #191): run `scripts/install-release.sh vX.Y.Z`, or `--from FILE` for a file you downloaded in a browser. It verifies the checksum and signature, will not replace a running Kai, and clears the stale Accessibility grant only when the code changed.
- Each release also carries `updater-Kai-X.Y.Z-darwin-arm64.zip`, a byte-identical copy of the zip under the file name the in-app updater requires (#196, #212); the installer ignores it. It does not make updates work by itself: installed apps still find no update while the repository is private.
- For developers: a build can be signed with a stable local certificate (`KAI_SIGN_IDENTITY="Kai Dev"`) so macOS permission grants survive rebuilds instead of resetting on every one (#192, #219); see `docs/dev-signing.md`. Release builds are still signed ad-hoc.
- A `LICENSE` file (upstream Kai's MIT text) is now in the repository (#198).

### Bug Fixes
- Settings > Shortcuts: the Screen Recording **Grant access** button now asks macOS for the permission before opening the pane. Before, it only opened the pane, and macOS lists an app there only after the app has asked, so Kai never appeared in the list and there was nothing to switch on. (Screenshot translation needs this permission.) Confirmed on a real build: Kai now appears in the list and can be switched on (#217).
- Settings > Shortcuts showed the Accessibility and Screen Recording rows as "Granted" whatever the real permission was, because Kai read the permission answer wrongly and treated every reply as "granted" (a bug inherited from upstream Kai). The rows now show the real state, and the missing-permission message from #194 (Alt+A and the tray input entry) now appears when the permission is really missing (#215). An answer that cannot be read is never shown as "Granted" in Settings, and never blocks a capture. Confirmed on a real build: the rows show "Not granted" until Kai is granted, and the missing-permission message appears.
- Controls that show Kai's own instant tooltip (the correction checkbox, the pin and auto-clipboard buttons, the three text-size buttons) no longer also show a second, native macOS tooltip, and the correction checkbox's tooltip is no longer pushed off the window's left edge (#214).
- Long toast messages (the missing-permission and double Cmd+C permission messages) are drawn as a rounded box that wraps its text, not as an ellipse with the text spilling out of it (#216).
- The permission messages (the Accessibility row's note, both toasts and the note under the double Cmd+C switch) are one short sentence each instead of four or five lines (#218).

### Known Issues
- Every new build of Kai is a new identity to macOS, and release builds are signed ad-hoc, so after installing one the Accessibility grant (and Screen Recording, and Input Monitoring) must be granted again. Kai says so when Accessibility is missing (#194, #215). Developers can avoid it with a stable local certificate (#192, #219); a downloaded release cannot.
- The in-app updater finds no update, whatever is released, until releases are readable without a login: this fork's repository is private (#196, #178). Install each release by hand or with `scripts/install-release.sh`.
- The Alt+A, tray and auto-clipboard paths write the text you copied or selected into Kai's own log (`~/.kai/logs/kai.log`) at info level. Nothing is sent anywhere, but treat that log as containing what you copied (#203).
- Correct grammar and wording is unproven on real Mexican Spanish. In the one check, Apple's model fixed a missing accent and replaced a mixed-in English phrase, but left a wrong verb tense ("nosotros vamos" for "fuimos") alone, so it does not catch every grammar error (#208).
- Screenshot translation (Alt+S) text does not go through the automatic source switch or the grammar correction (#200, #208).
- Translate on double Cmd+C ignores a real Cmd+C of yours made within half a second of Kai's own copy for Alt+A, and the password-manager list is best effort (Dashlane's bundle IDs are unconfirmed). It was tried once in the owner's testing and not in every app (#199).
- Running an older Kai (for example 0.1.0) against the same `~/.kai` settings folder as 0.2.0 appears to have reset 0.2.0's switches (translate on double Cmd+C, correct grammar). One observation, cause not proven.
- Alt+A can still miss selected text in Chrome (for example a Google Sheets cell). A probe found the simulated Cmd+C fails against a Chrome text field at every key delay tried. Not re-tested on 0.2.0.
- After shrinking the translate window, quitting and relaunching, one report said the window did not appear (possibly behind other windows). It was not reproduced, and not re-tested on 0.2.0 (#175).
- Apple's on-device engine can add blank lines between translated lines of multi-line input. Believed to be Apple's own behavior; not confirmed against another engine, and not re-tested on 0.2.0 (#173).
- The Baidu engine keeps only the first entry of Baidu's response (`internal/engine/baidu.go`), so if Baidu returns one entry per line, multi-line text loses every line after the first. Baidu's real response was not tested (#146).
- The Settings window was reported to disappear when Kai loses focus. That was filed against the old menu-bar-only mode; the Dock icon change (#165) may have fixed it, and it has not been re-tested on 0.2.0 (#154).

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
- Every new build is a new identity to macOS, so the Accessibility grant must be removed and added again after installing one. Kai now says so when the permission is missing (see #194 under [Unreleased]); the stable-signing fix is #192.
