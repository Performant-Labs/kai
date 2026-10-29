#!/usr/bin/env bash
# The project's unit test suite, as the Aftersight pipeline's seed runs it
# (.aftersight/project.json names this script as one plain command).
set -euo pipefail
(cd pkg/swiftbridge/scripts && bash ./build.sh)
go test -vet=off ./internal/... ./pkg/... -count=1
pnpm --dir frontend test
bash scripts/release.test.sh
bash scripts/release-ask-ai.test.sh
