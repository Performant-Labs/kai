[U] #161 UI walkthrough handoff (2026-09-28)

## T precondition
Met. handoff-T-green.md verdict PASS, and handoff-A-dup.md verdict PASS. I re-ran the authoritative command from CLAUDE.md at HEAD 348bb4f with `NODE_OPTIONS=--no-experimental-webstorage` at 8:18 AM MDT. It exits 0: the swiftbridge build, `go test ./internal/... ./pkg/...` all ok, and vitest 29 files, 396/396.

## Run environment
- Headless only. This is a Wails desktop app, and issue #28 says a live app can't be driven by synthetic input. I opened no browser, preview or screenshot.
- The Playwright MCP isn't needed and isn't connected in this session. The "real browser" stand-in is a jsdom mount of the real `TranslateWindow.svelte`, compiled by `@sveltejs/vite-plugin-svelte`. Only the Wails runtime and the generated bindings are mocked; they are the backend boundary.
- Harness: F's scratchpad mount (`161-mount/`), re-run unchanged at HEAD (the T-green tree), plus three U checks. For those checks the mocks were widened to two engines (google, deepl).
- Serve and cleanup: I copied the harness into `frontend/src/components/scratchU161.mount.ts` with `frontend/vitest.scratchU161.config.ts` and ran `npx vitest run -c vitest.scratchU161.config.ts`. Both files were then deleted, and `git status` is clean. No account or global state was touched. SaveConfig and Learn are mocks that only record calls.

## Wireframe conformance (revision 3, approved 2026-09-28)
Read against wireframe.html and handoff-D, from the markup and i18n at HEAD, and confirmed in the mount:
- Auto entry (1a-1d). `fromOptionLabel` returns `t('translate.sourceAuto')` for `auto` and `langName(value)` for every other value. It reads "Detected" / "自动检测" in every state, and no option ever carries a language-name suffix. `translate.detected` and `detectedLang.ts` are gone. `lang.auto` ("Auto") is untouched, and its only remaining user is `GeneralTab.svelte:79`, as the wireframe requires.
- Note markup (2b, 2c). The note is `<p class="u-muted px-4 text-[11px] first:pt-4" data-testid="detected-from-note">` and renders `translate.translatedFromDetected`:
  - en-US: "Translated from auto-detected {lang}"
  - zh-CN: "译自自动识别的{lang}"

  These are the approved wording for Q0/Q1. The size is 11 px (Q2).
- Rendering rule. The note shows when `detected_from` is set and `identity` is false (Q3: suppressed on identity). The frontend makes no comparison. The note is first in the note stack (Q5). It sits outside the textarea, so Copy never includes it.
- Swap (Q4). `swapLanguages` reads `detectedFrom` only while the source is auto. A pinned pair exchanges the pin, and that is unchanged.
- Short pastes (Q6). Under 20 code points after trimming, the pin is sent as given (`minPinCheckRunes`), with no note. This is the decided scope.

## Per-control checklist (action → expected → observed)
| # | Action | Expected | Observed | |
|---|---|---|---|---|
| A1 | Open with default_from auto | select=auto, Auto reads "Detected", no note | as expected; options `Detected, English, Spanish (Mexico), …` | PASS |
| A2 | Translate; result `detected_from: es-MX` | note "Translated from auto-detected Spanish (Mexico)", before textarea; select unmoved; no save/learn | as expected; class string exact | PASS |
| A3 | Click Copy | clipboard = result text only | `["你好，你好吗？"]` | PASS |
| A4 | Switch locale zh-CN | note "译自自动识别的西班牙语（墨西哥）", Auto "自动检测" | as expected | PASS |
| A5 | Identity result with `detected_from` set | identity note only | identity note shown, no detected note | PASS |
| A6 | Auto result, no detection | no note, Auto "Detected" | as expected | PASS |
| P1 | Open with default_from es-MX | select shows "Spanish (Mexico)", Auto "Detected" | as expected | PASS |
| P2 | Translate English; backend `detected_from: en` | note "…English"; select stays es-MX; no SaveConfig, no Learn | as expected | PASS |
| P3 | Translate again | request `from: es-MX` (the pin was not re-pinned) | `es-MX` | PASS |
| P4 | Pin stood (no detected_from) | no note | as expected | PASS |
| U1 | Result with note + phonetic + cancelled | stack: note, phonetic, Cancelled, textarea | `P:Translated from auto-detected English`, `SPAN:kuài`, `SPAN:Cancelled`, `TEXTAREA` | PASS |
| U2 | Engine dropdown google→deepl→google | note follows active engine (deepl pin stood: none; google: shown); pin unmoved | as expected | PASS |
| U3 | ⇄ after a pinned correction | exchanges the pin (zh ↔ es-MX), not the detection; no Learn | from=zh, request to=es-MX | PASS |

The console showed no errors. The only compile output is the existing a11y warning at `TranslateWindow.svelte:1152` (the pane divider), which this change does not touch.

## State matrix cells for the principal's live hand test
Headless can't reach these cells:
1. A real Apple call with a genuinely wrong pin. Pin es-MX, paste 20 or more code points of non-Spanish, non-target text (for example French, with target zh). Expect a real translation, not an echo, with the note naming the detected language, and the dropdown still on Spanish (Mexico). This proves substitute-as-auto on the real bridge. The backend was only exercised through loopback and scripted engines.
2. The identity case, which is the reported bug's shape. Pin es-MX, target en, paste 20 or more code points of English. Expect the identity note only, no "Translated from…" line, and the dropdown unmoved.
3. The note's real rendering in WKWebView. Check that it's muted, 11 px and on one line at 960 and 780 px, in light and dark, and in en-US and zh-CN. Check its spacing when it's the first line (`first:pt-4`) and when it's stacked above a phonetic line.
4. Short paste control. A pin mismatch under 20 code points still echoes silently, as the decided scope says.
5. The native macOS select. Check that the Auto entry and the closed select read "Detected" / "自动检测", and that Settings > General's interface-language option still reads "Auto".

## Findings
None blocking. Carried over, and none of these are UI defects:
- A-dup W2: the `<!-- prettier-ignore -->` above the note is stale since T-green's `,?` regex. It's cosmetic.
- The `TranslateCard` screenshot re-translate shows no note (known gap, AC (a)7). The PR body flags it.
- Deviation 1: a pin its detection confirms is never identity. It needs the principal's awareness, per T-green.

## Evidence
- The vitest mount output above: 13/13 pass, with the per-check `[A1]…[U3]` log lines.
- The authoritative suite run above.
- The source at HEAD: `TranslateWindow.svelte` :532-540 (`fromOptionLabel`) and :1300-1314 (the note), `en-US.ts` / `zh-CN.ts` / `keys.ts`, and `service.go` :271-321.

## Verdict
PASS (headless conformance to wireframe revision 3). The live cells above are left for the principal's hand test.

U complete, UI verified. Ready for S.
