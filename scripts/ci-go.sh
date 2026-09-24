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

# 2. Generate the frontend dist placeholder that main.go's
#    `//go:embed all:frontend/dist` needs to COMPILE (the real build is not run
#    here — see the lint-frontend note in ci.yaml).
mkdir -p frontend/dist && touch frontend/dist/index.html

# 3. Wails bindings (frontend/bindings) + icons + build-assets. The i18n merge
#    runs after these so the merged `*.json` (which embed pulls in) is current.
echo "==> wails generate (bindings icons build-assets)"
make -o tool-deps wails3-generate

# 4. sqlc-generated Go code.
echo "==> sqlc generate"
make -o tool-deps sqlc

# 5. Merge the split i18n JSON into the single embedded `*.json` files.
echo "==> i18n merge"
make -o tool-deps i18n

# 6. Build the real frontend dev bundle into frontend/dist. `//go:embed
#    all:frontend/dist` embeds whatever is in that dir at compile time, so
#    lint-frontend / golangci-lint / go test all see a real bundle, exactly like
#    push.yml (the only difference: push.yml's `pnpm build` runs after its
#    format step; here the frontend build runs before the Go checks so the
#    embed is populated for them).
echo "==> frontend build (real dist)"
( cd frontend && $PNPM run build:dev )

# 7. Lint: the NON-fixing golangci-lint (report only — see header).
echo "==> golangci-lint run (no --fix)"
make lint-go

# 8. Test.
echo "==> go test (internal/...)"
make test-go

# 9. Fuzz: mirror `make fuzz-go` (a 30s run of a fuzzer) but do NOT inherit
#    make's `FUZZ`/`TIME` env vars — `fuzz -fuzz=""` is an invalid target and
#    would hard-fail. Discover the fuzz targets by scanning the test files for
#    `func Fuzz...` (a build would compile them anyway, so this is cheap and
#    authoritative). If any exist, run the first for 30s like push.yml; if
#    none exist, the no-op default target passes trivially, so say so and move
#    on instead of inventing a target that make would reject.
FUZZ_TARGETS="$(grep -rhoE 'func (Fuzz[A-Za-z0-9_]+)\(' --include=*_test.go ./internal/ 2>/dev/null | sed -E 's/func ([A-Za-z0-9_]+)\(.*/\1/' | sort -u || true)"
if [ -n "$FUZZ_TARGETS" ]; then
  FUZZ="$(echo "$FUZZ_TARGETS" | head -1)"
  echo "==> go fuzz (first of: $(echo $FUZZ_TARGETS | tr '\n' ' '); running $FUZZ for 30s)"
  go test -vet=off -fuzz="$FUZZ" -fuzztime=30s ./internal/...
else
  echo "==> no Fuzz* targets found under ./internal/...; fuzz step is a no-op (matches make fuzz-go's empty default) — skipping"
fi

# 10. Vulnerability scan.
echo "==> govulncheck (no --fix)"
make vuln-go

# 11. Format CHECK: the non-fixing counterpart of `make format`. A dirty
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

echo "==> ci-go.sh: all Go checks passed"
