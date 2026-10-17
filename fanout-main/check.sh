#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
go test -count=1 -timeout=120s ./...
go vet ./...
bash -n install.sh
bash -n f.sh
node --check web_check.js
node --check worker/worker.js
node frontend_test.cjs
if [[ "${FANOUT_RACE:-0}" == 1 ]]; then go test -race -timeout=180s ./...; fi
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "${TMPDIR:-/tmp}/fanout-check-amd64" .
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o "${TMPDIR:-/tmp}/fanout-check-arm64" .
