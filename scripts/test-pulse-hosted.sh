#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"
docker run --rm -v "${ROOT}:/src" -w /src golang:1.25-alpine \
  go test ./internal/analytics/ -count=1 -run 'PulseBeta|ParsePulse'
