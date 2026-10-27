#!/usr/bin/env bash
# The authoritative unit suite (RED/GREEN): .aftersight/project.json runs this file, which
# stays a plain program because the seed forbids shell characters in the command.
set -euo pipefail
cd "$(dirname "$0")/.."

(cd pkg/swiftbridge/scripts && bash ./build.sh)
go test -vet=off ./internal/... ./pkg/... -count=1
pnpm --dir frontend test
bash scripts/release.test.sh
bash scripts/release-ask-ai.test.sh
