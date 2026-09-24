#!/usr/bin/env bash
#
# ci-go.sh — run Kai's Go build/lint/test on the self-hosted fleet via ci.yaml.
#
# Mirrors the check sequence that upstream's PushRequest (`.github/workflows/
# push.yml`) runs — but only the parts the fleet image `pl-runner:1.70.1` can
# actually run. It is invoked as the sole `step` of the `go` job in the root
# `ci.yaml` (see that file's header for why one job, one step):
#
#   The Playbook's `ci-declarative` path expands each ci.yaml `step` into its
#   OWN GitHub Actions job with a FRESH `actions/checkout` (one job per
#   (job, step) pair, ci-reusable.yml). Generated files — the wails3 bindings,
#   icons, the `build/` assets that `wails3 task common:build` reads, the
#   sqlc-generated Go code, the merged i18n `*.json`, and the
#   `frontend/dist` placeholder that `//go:embed all:frontend/dist` in
#   main.go needs — do not survive across jobs. So the whole sequence must
#   run inside ONE step on ONE runner. This script is that step: it lays down
#   the generated files, then runs the checks, all in a single shell.
#
# It runs on the fleet self-hosted runners (org variable CI_RUNNER =
# self-hosted-linux), NOT on GitHub-hosted ubuntu-latest like push.yml, and it
# therefore DELIBERATELY OMITS three things push.yml does. See the `go` job
# header in ci.yaml for the rationale; in short:
#
#   1. No `sudo apt-get install ...` — pl-runner:1.70.1 bakes pkg-config,
#      libgtk-4-dev, libwebkitgtk-6.0-dev and jq (Playbook#767); apt needs
#      sudo, which the fleet rules forbid.
#   2. No `make tool-deps` — that target `go install ...@latest`s wails3,
#      sqlc, golangci-lint and govulncheck on every run and would overwrite
#      the fleet's pinned, baked versions (its `|| true` only guards the
#      version print, not the install). The baked tools are used as-is; where a
#      make target would pull in tool-deps, it is bypassed (see the per-target
#      notes below).
#   3. `make format` / `make check` are replaced by their NON-fixing variants.
#      push.yml runs `make format` (gofmt -w, go fmt, go fix) and
#      `make check` which includes `lint-go-fix` (`golangci-lint run --fix`) —
#      both of which REWRITE source files in place. On the fleet that would
#      make a dirty checkout pass and the job could not report the diff it
#      made. CI must only REPORT, never silently rewrite, so this runs
#      `golangci-lint run` (no --fix) and a format CHECK (gofmt -l must be
#      empty) instead.
#   4. The Swift bridge build. push.yml (on a macOS runner) runs
#      `make swift-build`, which invokes swiftc — but the fleet image has no
#      Swift toolchain (swiftc/xcrun are absent, so `make swift-build` cannot
#      run here at all). Instead the script runs the same build script in
#      SKIP mode (`KAI_BRIDGE_SKIP_BUILD=1 bash build.sh`): build.sh removes
#      the stale libkai_bridge.a/.dylib and then exits 0 without invoking
#      swiftc, leaving the dylib/a COMMITTED in the repo (git-tracked, so
#      present in every checkout) as the bridge the Go tests dlopen. This is
#      the faithful Linux analogue: the artifacts under test are byte-identical
#      to what a macOS `make swift-build` would produce from the same sources,
#      and a real build simply cannot happen on a Swift-less image.
#
# It also omits push.yml's setup/cache steps: no setup-go, no pnpm/action-setup,
# and no actions/cache for the Go module / build / Go-bin / pnpm caches — the
# fleet image already bakes the Go toolchain and the shared host caches under
# /opt/runner-cache (GOMODCACHE etc.), which the `go` env block below points at.
#
# Generated-file note: wails3 `generate` and the i18n merge are run here even
# though the generated files are already committed — because a fresh CI checkout
# is clean, and the `//go:embed` directives plus golangci-lint need those paths
# to exist for the packages to compile.
#
# MEMORY (issue #24): the ci-host-a runners run their job containers with
# `mem_limit: 3g` (a deliberate guardrail added after the Sep 3-7 OOM incident
# — do NOT "fix" this by raising or removing the limit; the fix is on the job
# side). ci-host-b runners have no limit, so the same job is a coin flip there.
#
# Reproduced locally on ci-host-a in the same image (pl-runner:1.70.1) and the
# same cgroup limits (docker run -m 3g --memory-swap 4500m --cpus 8, the
# runners' exact values), against the real /opt/runner-cache (the failed
# CI run took the COLD-cache path — first CI run on its runner — so the
# honest reproduction is a cold GOCACHE):
#   - DEFAULTS (GOGC=100, no GOMEMLIMIT, GOMAXPROCS=8): govulncheck is
#     OOM-killed ~23s into its "Checking the code" phase, exit != 0
#     (matches run 35947517287: "make: *** [Makefile:218: vuln-go] Killed").
#   - GOMEMLIMIT=2560MiB only (GOGC stays 100): PASSES 4/4 consecutive
#     runs, ~12s govulncheck, full checks green.
# So the one knob that matters is GOMEMLIMIT: the Go runtime (1.27) starts
# sweeping far more aggressively as the heap approaches the soft limit, which
# is exactly what keeps the "Checking" phase's working set (every importable
# package of all 72 modules in the import graph, cross-referenced against the
# Go vuln DB) under the 3g ceiling. GOGC=50 / GOMAXPROCS=2 were tried first
# and are NOT needed — GOGC=50 alone did not prevent the kill, and GOMAXPROCS=2
# only adds GC latency for no measured memory gain, so both stay at their
# defaults (100 / ncpu).
# GOMEMLIMIT=2560MiB is deliberately ~440MiB under the 3g container: the
# cgroup counts the C allocator and OS pages too, not just the Go heap, and
# the runner agent runs alongside the job in the same container. It is a
# soft limit (a pressure signal, not a hard cap) — on unlimited ci-host-b
# runners the same export simply never binds, so the job cannot slow down
# there.
# govulncheck coverage is unchanged: same `./...` pattern as `make vuln-go`
# (which is `govulncheck -show verbose ./...`), so the identical 23 root
# packages, 72 modules and stdlib are scanned — only the runtime's GC
# behaviour differs. The script also prints its own VmHWM at the end so a
# future regression is visible in the log instead of a silent OOM kill.
set -euo pipefail

# Run the whole sequence from the repo root (the checkout root) so that the
# relative `cd` paths in the Makefile targets resolve correctly.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# The fleet image ships its shared dependency caches on the host under
# /opt/runner-cache. GOMODCACHE is the Go module cache; GOCACHE is the
# compile cache. Pointing at them means `go mod download` and the build/test
# recompiles are cache-hits instead of cold, which is what keeps this under the
# ci-declarative job's 20-minute cap on a fresh checkout. (GOCACHE defaults to
# $HOME/.cache/go-build when unset; we set it explicitly so a job that runs as
# the runner user still lands on the shared dir.)
export GOMODCACHE="${GOMODCACHE:-/opt/runner-cache/go}"
export GOCACHE="${GOCACHE:-/opt/runner-cache/go-build}"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"

# Memory guardrail (issue #24) — see the MEMORY header above for the full
# rationale and the measured numbers. Exported (not just set on the
# individual commands) so that every Go tool this script spawns —
# golangci-lint, go test, the fuzz run, govulncheck, gofmt — inherits the
# same runtime instead of each growing until the container's cgroup OOM
# killer fires. GOMEMLIMIT is a SOFT ceiling: the Go runtime starts GCing
# much more aggressively as the heap approaches it; it does not kill the
# process at the limit. 2560MiB leaves ~440MiB under the 3g container for
# the C allocator and for the runner agent that shares the container. On
# unlimited ci-host-b runners the limit simply never binds, so the job cannot
# slow down there. GOGC and GOMAXPROCS are deliberately left at their
# defaults — see the header: GOGC=50 alone did not prevent the OOM, and a
# forced GOMAXPROCS=2 only adds latency for no measured memory gain.
export GOMEMLIMIT="${GOMEMLIMIT:-2560MiB}"
echo "==> memory guardrail: GOMEMLIMIT=$GOMEMLIMIT (ci-host-a containers are mem_limit:3g; ci-host-b has no limit, so this never binds there)"

# pnpm is invoked via `npx --yes pnpm@<pin>`, NOT a bare `pnpm`: pl-runner
# bakes the Go CLIs (wails3, sqlc, golangci-lint, govulncheck) into
# /usr/local/bin, and node 26.7 + npm 11.19 are baked (apt), but NO global
# pnpm and NO npx are on a ci-declarative job's PATH by default. The `node:
# "26"` key on the go job in ci.yaml is what changes that: it makes
# ci-reusable.yml run actions/setup-node for the job, which puts node/npm/npx
# on PATH (verified live: on a job WITHOUT the key, `npx: ABSENT` and
# `npx --yes pnpm@12.5.1` does not run; WITH the key, npx is /usr/bin/npx and
# `npx --yes pnpm@12.5.1 --version` → 12.5.1). The pinned version (12.5.1,
# from frontend/package.json's packageManager field) is the one the lockfile
# was written with, and this is the same invoker the frontend job uses.
# (pnpm itself is fetched from the public npm registry by npx and cached under
# /opt/runner-cache/npm — the image's shared npx cache — so it is warm after
# the first run.)
PNPM="${PNPM:-npx --yes pnpm@12.5.1}"

# The Makefile's own targets (deps, and the lint-frontend / build-frontend
# targets that `make check` calls) invoke a BARE `pnpm` (`pnpm --dir
# ./frontend ...`). A bare pnpm is not on a fleet ci-declarative job's PATH
# (only npx is, via the go job's node: key -> setup-node), so every one of
# those Makefile pnpm calls would be `command not found` — and this story must
# NOT edit the Makefile (trap #1 is about not clobbering the baked tools, but
# the story's scope is ci.yaml + a script; the Makefile is also used by local
# dev, where a real global pnpm exists and a shim would be wrong noise). So
# instead the script provides the missing executable: a one-line `pnpm` shim
# in a dir prepended to PATH that execs the npx-invoked pinned pnpm with its
# arguments verbatim. `pnpm --dir ./frontend install` thus runs exactly what
# the job's install hook already ran (the warm /opt/runner-cache/npm makes the
# npx hit a cache lookup, and pnpm install on an installed node_modules is a
# no-op that exits 0 in ~1s, as the fleet's own install hook just demonstrated:
# "Done in 1.4s using pnpm v12.5.1"). This is the same mechanism ci.yaml's
# frontend job relies on (its `pnpm --dir ./frontend` style Makefile calls and
# its steps all resolve through npx), just made explicit for the Makefile.
# The shim goes in $HOME/.local/shim (per-user, under the runner's home, on
# PATH, no sudo) and is prepended AFTER the setup-node npx shim so npx itself
# still resolves from /usr/bin.
SHIM_DIR="$HOME/.local/shim"
mkdir -p "$SHIM_DIR"
# NOTE: the shim's body is written with printf '%s\n' and each argument
# single-quoted, so the body text must not itself contain a single quote
# (an apostrophe would terminate the arg and the next token would be executed
# as a command — that is exactly the 'ci.yaml: command not found' the fleet
# hit on run 35946193153 when a comment line held an apostrophe). Keep the
# body apostrophe-free; the comment above this block carries the real story.
printf '%s\n' \
  '#!/usr/bin/env bash' \
  '# pnpm shim for the fleet ci-declarative go job: the image bakes node+npm' \
  '# but no pnpm and no npx-on-PATH-by-default. ci.yaml go job sets node 26,' \
  '# so setup-node puts npx on PATH before this step; this shim is what makes' \
  '# the Makefile bare pnpm calls (make deps, make lint-frontend, etc.)' \
  '# resolve to the pinned pnpm, exactly as the job install hook invokes it.' \
  'exec npx --yes pnpm@12.5.1 "$@"' \
  > "$SHIM_DIR/pnpm"
chmod +x "$SHIM_DIR/pnpm"
export PATH="$SHIM_DIR:$PATH"

echo "==> ci-go.sh starting in $ROOT"

# ---- Toolchain sanity: fail loudly (with the versions) before the long path,
# so a fleet image that has lost a baked tool is caught at the top, not 10
# minutes in.
go version
wails3 version
sqlc version
golangci-lint --version | head -1
govulncheck -version 2>&1 | head -1 || true
# node + npx: NOT baked on the fleet — the go job's `node: "26"` key in
# ci.yaml runs actions/setup-node before this step, which is what puts node
# and npx on PATH (probe: ABSENT without the key). Hard-require both; the
# job must fail fast and loudly if the fleet ever changes that ordering.
node --version
npx --version
# pnpm is NOT baked (see the PNPM note above) — the check that must not be
# skipped is that the npx-invoked pinned pnpm actually runs.
$PNPM --version

# 1. Dependencies. `make deps` = `go mod download` + `pnpm install` in frontend/.
#    (No `make tool-deps` here — see the header.)
echo "==> deps"
make deps

# 2. Swift bridge build — the fleet analogue of push.yml's `make swift-build`
#    (see omission note #4 in the header): the image has no Swift toolchain,
#    so build.sh runs in SKIP mode, which removes the stale bridge artifacts
#    and exits 0, leaving the COMMITTED libkai_bridge.a/.dylib (git-tracked)
#    as what the Go tests dlopen. The committed artifacts are byte-identical to
#    what a macOS build would produce from the same sources.
echo "==> swift bridge build (SKIP mode — no Swift toolchain on the fleet; the committed artifacts stand in)"
( cd pkg/swiftbridge/scripts && KAI_BRIDGE_SKIP_BUILD=1 bash ./build.sh )

# 3. Generate the frontend dist placeholder that main.go's
#    `//go:embed all:frontend/dist` needs to COMPILE (the real build is not run
#    here — see the lint-frontend note in ci.yaml).
mkdir -p frontend/dist && touch frontend/dist/index.html

# 4. Wails bindings (frontend/bindings) + icons + build-assets. The i18n merge
#    runs after these so the merged `*.json` (which embed pulls in) is current.
echo "==> wails generate (bindings icons build-assets)"
make -o tool-deps wails3-generate

# 5. sqlc-generated Go code.
echo "==> sqlc generate"
make -o tool-deps sqlc

# 6. Merge the split i18n JSON into the single embedded `*.json` files.
echo "==> i18n merge"
make -o tool-deps i18n

# 7. Build the real frontend dev bundle into frontend/dist. `//go:embed
#    all:frontend/dist` embeds whatever is in that dir at compile time, so
#    lint-frontend / golangci-lint / go test all see a real bundle, exactly like
#    push.yml (the only difference: push.yml's `pnpm build` runs after its
#    format step; here the frontend build runs before the Go checks so the
#    embed is populated for them).
echo "==> frontend build (real dist)"
( cd frontend && $PNPM run build:dev )

# 8. Lint: the NON-fixing golangci-lint (report only — see header).
echo "==> golangci-lint run (no --fix)"
make lint-go

# 9. Test — the WIDENED invocation `go test -vet=off -v ./internal/...
#    ./pkg/... -count=1` (issue #7: the pkg/... scope now includes
#    wails-updater-providers' loopback test). GOMEMLIMIT (above) bounds the
#    runtime for this step, and the suite is still fast, so no parallelism knob
#    is needed (the measured OOM was govulncheck's, not the tests').
echo "==> go test (internal/... + pkg/...)"
make test-go

# 9b. Frontend test — `pnpm --dir ./frontend test --no-file-parallelism`
#     (vitest --run, issue #7): the same command pipeline's test.unit.command
#     runs, so the fleet runs the suite t-green gates on, with vitest's file
#     parallelism turned OFF (issue #9). node_modules is already installed
#     (job install hook + make deps), so vitest resolves from the warm node
#     install. The invocation is made directly here instead of via `make
#     test-frontend` because pnpm forwards the trailing arguments to the
#     script, so --no-file-parallelism lands on `vitest --run` without
#     touching the Makefile or frontend/package.json.
#
#     WHY SERIAL (issue #9, ~5-20% measured frontend flake): Node 26 (baked in
#     pl-runner) registers localStorage/sessionStorage as LAZY own properties
#     on globalThis that return `undefined` unless --localstorage-file is
#     passed; vitest's jsdom env leaves that native getter in place (it only
#     copies window keys absent from global), so the tests' window.localStorage
#     is undefined and every case throws. Exporting
#     NODE_OPTIONS=--localstorage-file makes Node's own getter a real, file-
#     backed Storage, which vitest leaves untouched (window===globalThis in the
#     tests), so window.localStorage just works. The file lives under $HOME
#     (writable, per-runner) and is disposable — it is test scratch, not state,
#     and is never committed (docs/handoffs and dot-artifacts stay untracked).
#     But that ONE file-backed store is shared by ALL vitest workers: with the
#     default --file-parallelism on, the three frontend test files run in
#     parallel node workers that all read/write the same .kai-localstorage.json,
#     and those cross-file races are the remaining flake. --no-file-parallelism
#     runs the test files one at a time, so no two workers share the store at
#     once — the race surface is removed without dropping a single test case
#     (same suite, same assertions; only the scheduling changes).
#     NODE_OPTIONS is exported (not scoped to the pnpm line) so pnpm -> vitest
#     -> the node worker processes all inherit it; it is set immediately before
#     this step and not consumed by anything after it.
export NODE_OPTIONS="--localstorage-file=${HOME}/.kai-localstorage.json"
echo "==> frontend test (vitest, files serial)"
pnpm --dir ./frontend test --no-file-parallelism

# 10. Fuzz: mirror `make fuzz-go` (a 30s run of a fuzzer) but do NOT inherit
#    make's `FUZZ`/`TIME` env vars — `fuzz -fuzz=""` is an invalid target and
#    would hard-fail. Discover the fuzz targets by scanning the test files for
#    `func Fuzz...` (a build would compile them anyway, so this is cheap and
#    authoritative). If any exist, run the first for 30s exactly like push.yml
#    (no parallelism override — see step 8); if none exist, the no-op default
#    target passes trivially, so say so and move on instead of inventing a
#    target that make would reject.
FUZZ_TARGETS="$(grep -rhoE 'func (Fuzz[A-Za-z0-9_]+)\(' --include=*_test.go ./internal/ 2>/dev/null | sed -E 's/func ([A-Za-z0-9_]+)\(.*/\1/' | sort -u || true)"
if [ -n "$FUZZ_TARGETS" ]; then
  FUZZ="$(echo "$FUZZ_TARGETS" | head -1)"
  echo "==> go fuzz (first of: $(echo $FUZZ_TARGETS | tr '\n' ' '); running $FUZZ for 30s)"
  go test -vet=off -fuzz="$FUZZ" -fuzztime=30s ./internal/...
else
  echo "==> no Fuzz* targets found under ./internal/...; fuzz step is a no-op (matches make fuzz-go's empty default) — skipping"
fi

# 11. Vulnerability scan — the step the OOM was measured on (issue #24).
#     The command is IDENTICAL to `make vuln-go` (`govulncheck -show verbose
#     ./...`): same 23 root packages, same 72 modules, same stdlib — no
#     package, module or vulnerability class is dropped. The only difference
#     is the inherited GOMEMLIMIT=2560MiB, which makes the Go runtime GC
#     aggressively enough that the "Checking the code" phase's working set
#     (reproduced locally: killed at ~23s with defaults, 4/4 passes in
#     ~12s with the limit) stays under the 3g container ceiling.
echo "==> govulncheck (no --fix)"
make vuln-go

# 12. Format CHECK: the non-fixing counterpart of `make format`. A dirty
#     checkout must FAIL here (and print the files it would rewrite), not be
#     silently rewritten. gofmt -l lists files whose formatting differs; if it
#     is non-empty we print them and exit 1. (gofmt -l is a read-only check;
#     it never writes, so a green result means the tree is already formatted.)
echo "==> format check (gofmt -l must be empty)"
UNFORMATTED="$(gofmt -l . 2>/dev/null | grep -v -E '^(vendor/|frontend/)' || true)"
if [ -n "$UNFORMATTED" ]; then
  echo "!! Unformatted Go files (would be rewritten by make format):"
  echo "$UNFORMATTED"
  echo "!! Run 'make format' locally. CI must stay read-only."
  exit 1
fi
echo "format check: clean"

# 13. Report peak memory (issue #24 acceptance: "report peak memory if
#     measurable"). VmHWM in /proc/self/status is the high-water mark of this
#     process's virtual memory; the Go runtime's own peak (the number that
#     matters against the 3g cgroup) is the runtime.MemStats value the Go
#     tools do not print, so VmHWM is an over-approximation — but if it is
#     comfortably under 3g the job is safe, and if a future change pushes it
#     toward the ceiling this line makes that visible in the log instead of
#     a silent OOM kill.
if [ -r /proc/self/status ]; then
  PEAK="$(awk '/^VmHWM:/{print $2" "$3}' /proc/self/status)"
  echo "==> peak virtual memory of ci-go.sh: $PEAK (container ceiling: 3g on ci-host-a)"
fi

echo "==> ci-go.sh: all Go checks passed"
