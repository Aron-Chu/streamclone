#!/usr/bin/env bash
# Streamclone public product boundary grep gate (Step 7 strict).
# See docs/streampulse-product-boundary.md.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "${ROOT}"

MODE="${1:---preflight}"
STRICT="${STREAMCLONE_BOUNDARY_STRICT:-0}"
if [[ "${MODE}" == "--strict" ]]; then
  STRICT=1
fi

RG=(rg -n --no-heading
  'packages/pulse|packages/analytics-console|packages/pulse-charts|cmd/analytics|internal/analytics/|test-analytics|profile-bearhost|profile-hosted|PULSE_|api\.streampulse\.stream|production-promotion|hosted-production|ingest-phase|/v1/extension|/v1/portal|/v1/analytics/|/v1/admin/archive|pulse-live-coverage|ingest-core|storygraph|pulse-wire|make up-scraper|@streamclone/pulse|VITE_ANALYTICS_URL|VITE_REPLAYFORGE_UI_URL|REPLAYFORGE_UI|Start ReplayForge|StudioRedirect|ClipStudio|ArchiveAdminPage|useAnalyticsLive|watchAnalyticsChannel|getAnalyticsLive|analytics-workers|Caddyfile\.pulse-api|route @clipper|path /v1/clipper'
  --glob '!docs/streampulse-product-boundary.md'
  --glob '!docs/agents-streamclone-and-replayforge.md'
  --glob '!docs/clipper-responsibility-allowlist.md'
  --glob '!docs/options.md'
  --glob '!AGENTS.md'
  --glob '!CHANGELOG.md'
  --glob '!.cursor/plans/**'
  --glob '!scripts/check-product-boundary.sh'
  --glob '!scripts/pre-commit-product-boundary-guard.sh'
  --glob '!scripts/pre-commit-public-ops-guard.sh'
  --glob '!scripts/ci-public-topology-scan.sh'
  --glob '!scripts/ci-context-contract.sh'
  --glob '!scripts/_tmp_*.py'
  --glob '!docs/archive/**'
  --glob '!docs/agent-notes/**'
  --glob '!docs/storage/**'
  --glob '!docs/azure-archive-plane.md'
  --glob '!docs/agent-codegraph.md'
  --glob '!docs/agent-context.md'
  --glob '!.kiro/specs/**'
  --glob '!.kiro/steering/pulse-wire.md'
  --glob '!.kiro/steering/analytics.md'
  --glob '!.cursor/skills/streamclone/analytics-sync/**'
  --glob '!.cursor/skills/streamclone/clipper-local/**'
  --glob '!.agents/skills/streamclone/analytics-sync/**'
  --glob '!.agents/skills/streamclone/clipper-local/**'
  --glob '!.cursor/rules/clipper.mdc'
  --glob '!internal/boundaryguard/**'
  --glob '!frontend/tests/fixtures/**'
)

mapfile -t HITS < <("${RG[@]}" . 2>/dev/null || true)

# Directory existence lock (CI / local working tree).
DIR_HITS=()
for d in cmd/analytics packages/analytics-console packages/pulse-charts; do
  if [[ -d "${d}" ]]; then
    DIR_HITS+=("directory exists: ${d}")
  fi
done
# internal/analytics may only exist as empty leftover; fail if any .go files return.
if [[ -d internal/analytics ]] && find internal/analytics -name '*.go' -print -quit 2>/dev/null | grep -q .; then
  DIR_HITS+=("Go sources under internal/analytics/")
fi

ALL_HITS=("${HITS[@]}" "${DIR_HITS[@]}")

if [[ "${#ALL_HITS[@]}" -eq 0 ]]; then
  echo "check-product-boundary: OK (${MODE}, strict=${STRICT})"
  exit 0
fi

echo "check-product-boundary: ${#ALL_HITS[@]} hit(s) (${MODE}, strict=${STRICT})" >&2
printf '%s\n' "${ALL_HITS[@]}" | head -n 80 >&2 || true
if [[ "${#ALL_HITS[@]}" -gt 80 ]]; then
  echo "... and $((${#ALL_HITS[@]} - 80)) more" >&2
fi

if [[ "${STRICT}" -eq 1 ]]; then
  echo "Strict Step 7 gate failed. Trim scripts/docs/install/UI surfaces; do not re-add Analytics or ReplayForge." >&2
  exit 1
fi

echo "Preflight mode: hits reported only (STREAMCLONE_BOUNDARY_STRICT=1 or --strict to fail)." >&2
exit 0
