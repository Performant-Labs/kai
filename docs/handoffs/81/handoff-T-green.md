# handoff-T-green: #81 idle pane and retained session (Phase 7, GREEN + Tier 2)

## GREEN confirmation
- Test repair (T-owned): sessionRetention.test.ts "does not touch localStorage directly" matched the pre-existing #39 pin migration (identical on master), so it could not pass. Replaced with a session-scoped check (persisted<TranslateSession> present, key `kai:translate:session` appears once, no raw localStorage call mentions the session). Intent of AC4 preserved.
- Frontend (NODE_OPTIONS=--no-experimental-webstorage): 20 files, 177 tests passed.
- Go `go test -vet=off ./internal/... ./pkg/...`: all ok (only ld version warnings, no FAIL).
- `tsc --noEmit`: exit 0.

## Tier 1
Build/tsc ok; F's reported vite build, prettier and gofmt results accepted from re-run of tsc/tests; changed Go files are comment-only.

## Tier 2
- Coverage: each AC (idle, failed only after request, session model, persisted store, close retains, clear resets, restore robustness) has a pinning test at unit/integration/source-contract tier.
- Tests pin behavior via F's reported mutants killed by the committed tests (clear-on-close, idle-as-failed, dots ignore requested, Clear keeps requested, request never marked, persisting loading).
- restoreSession never throws, drops invalid entries, forces requested false; corrupt/wrong-shape falls back to empty.
- Security/data: source text and results now also in localStorage (same data class as history DB); Clear removes it.
- Comment-only edits in main.go, events.go, events.ts: benign.

## Acceptance criteria: all met, each backed by a passing test.

## Blocking issues: none.

## Advisory
- Source-contract tests cannot see reactive behavior; component-render harness (#78) would.
- Type-hole (Record<string, TranslateResult> vs PaneResult) is unchecked by CI (svelte-check off); pre-existing class.
- Idle dot look and real WKWebView persistence need the principal's hand test.
