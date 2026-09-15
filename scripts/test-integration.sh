#!/usr/bin/env bash
# Integration tests against a real PostgreSQL.
# RR_TEST_DATABASE_URL must point to a DEDICATED test database:
# the tests apply all migrations and roll them back to zero.
# Usage: RR_TEST_DATABASE_URL=postgres://... bash scripts/test-integration.sh
set -euo pipefail
cd "$(dirname "$0")/.."

: "${RR_TEST_DATABASE_URL:?RR_TEST_DATABASE_URL is not set (see .env.example)}"

go test -tags integration -count=1 ./tests/integration/...
