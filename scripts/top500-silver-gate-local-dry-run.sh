#!/usr/bin/env bash
# LOAD-003c: local-only selective silver gate dry-run (fixture candidates; no queue writes).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$ROOT/deploy/env/.env.silver-gate-local"

if [[ ! -f "$ENV_FILE" ]]; then
  cat >"$ENV_FILE" <<'EOF'
TOP500_SILVER_GATE_ENABLED=true
TOP500_SILVER_GATE_DRY_RUN=true
TOP500_SILVER_GATE_WRITE_ENABLED=false
TOP500_SILVER_GATE_MAX_CANDIDATES=5
TOP500_SILVER_GATE_MAX_ENQUEUE_PER_RUN=1
TOP500_SILVER_GATE_INTERVAL=10m
TOP500_SILVER_GATE_FIXTURE_CANDIDATES=true
EOF
  echo "Wrote gitignored local override: deploy/env/.env.silver-gate-local"
fi

echo "=== LOAD-003c local dry-run (fixture; in-process) ==="
docker run --rm -v "$ROOT:/src" -w /src golang:1.25-alpine \
  go test ./internal/analytics -run 'TestTop500SilverGateLocalDryRun' -count=1 -v

echo "=== Static side-effect scan (silver gate production files) ==="
docker run --rm -v "$ROOT:/src" -w /src golang:1.25-alpine \
  go test ./internal/analytics -run TestSilverGateProductionFilesHaveNoForbiddenSideEffects -count=1 -v

echo "PASS: local dry-run harness complete (no hosted run; no queue writes)"
