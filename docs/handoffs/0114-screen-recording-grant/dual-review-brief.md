## Brief Review (Round 1)

### BLOCK findings
None.

### Needs-verification findings
- **[NV-1]** Claim: "a failed `.notPermitted` `SCStream` start still enrolls the app with macOS TCC" (Settled decision 1, and the entire premise of AC1's escalation). This is a runtime macOS/TCC behavior claim with no evidence in this prompt. Evidence that would settle it: an Apple documentation excerpt on ScreenCaptureKit/`CGRequestScreenCaptureAccess` TCC enrollment semantics, or a cited documented case where starting a stream triggered the listing without user approval of a capture.
- **[NV-2]** Claim: `CGRequestScreenCaptureAccess()` followed by a capture attempt plus opening `Privacy_ScreenCapture` via `NSWorkspace` produces the Kai toggle appearing "without the remove/re-add dance" (Objective). No source in the prompt shows this combination causes registration. Evidence: Apple docs / a reproducible transcript on the target macOS version(s); also the macOS version range matters, since the pane renamed to "Screen & System Audio Recording" only in recent macOS — brief never pins a minimum OS.
- **[NV-3]** Claim: existing envelope pins ("`Thread.isMainThread` / `DispatchQueue.main`, `NSApp.activate`, pane URL `Privacy_ScreenCapture`") "keep holding" after the function is rewritten to include an `SCStream` start (AC2). SCStream start is async; whether the existing main-thread/once-guard pins remain compatible with an async start/stop is not shown. Evidence: current text of `kai_screenrecording_request` in `apple_accessibility.swift` and the existing pins in `screenrecording_source_test.go` (file:line excerpts).

### WARN findings
- **[W-1]** AC1 is ambiguous on the preflight-success path: it pins order "request → preflight check → attempt → pane", but does not say whether the attempt and pane-open still occur when `CGPreflightScreenCaptureAccess()` returns true (Kai already granted). F could implement either reading and pass the pin. Recommendation: state explicitly what happens when preflight succeeds (skip attempt? still open pane?).
- **[W-2]** "One real capture attempt (a started `SCStream`, stopped immediately; no frames retained)" is under-specified for a source-text pin: SCStream requires a content filter, configuration, and typically an output/delegate; "stopped as soon as it answers" needs a concrete signal (completion handler result? first frame?). T's RED pin will have to invent these semantics. Recommendation: name the exact call sequence to be pinned (`SCStream`, `startCapture`, `stopCapture`, on which queue, with what timeout) so F and T converge.
- **[W-3]** AC4's "no test wires a real request pointer" is only checkable by auditing the whole suite's bridge wiring; the brief names the `unwireBridgeRequests` pattern from #101/#102 but doesn't list which tests touch `KaiScreenRecordingRequest`. Recommend T enumerate the call sites in the test contract.

### NIT findings
- **[NIT-1]** AC3's en-US string contains an unescaped apostrophe-free phrasing but embeds an ampersand ("Screen & System Audio Recording"); confirm the permissionCopy ≤110 length check counts chars after any escaping, so the pin doesn't flake on encoding.

### Verdict
PASS — no BLOCK findings; implementation may proceed.