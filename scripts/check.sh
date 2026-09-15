#!/usr/bin/env bash
# Fast checks without a database: formatting, vet, unit and architecture tests.
# CI runs the same script. Usage: bash scripts/check.sh
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> gofmt"
unformatted="$(gofmt -l src tests migrations)"
if [ -n "$unformatted" ]; then
  echo "Files need formatting (run: gofmt -w src tests migrations):"
  echo "$unformatted"
  exit 1
fi

echo "==> go vet"
go vet ./...
go vet -tags integration ./...

echo "==> go test (unit + architecture)"
go test -count=1 ./...

echo "OK"
