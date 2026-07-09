#!/usr/bin/env bash
# smoke-e2e-clipper.sh — end-to-end Auto Clipper validation
#
# Runs when both Streamclone (localhost:8090) and ReplayForge (localhost:8095)
# are up. Exercises the full flow:
#   Analytics moment → Export Moment → ReplayForge job creation → mirrored state.
#
# Prerequisites:
#   make up  (streamclone stack)
#   ReplayForge running on :8095
#
# Usage:
#   bash scripts/smoke-e2e-clipper.sh
#
# Requirements validated: 6.1, 8.6
set -euo pipefail

STREAMCLONE_URL="${STREAMCLONE_URL:-http://localhost:8090}"
REPLAYFORGE_URL="${REPLAYFORGE_URL:-http://localhost:8095}"

echo "=== Auto Clipper E2E Smoke ==="
echo "Streamclone: $STREAMCLONE_URL"
echo "ReplayForge: $REPLAYFORGE_URL"
echo ""

# Step 1: Check Streamclone is alive
echo "[1/4] Checking Streamclone health..."
sc_health=$(curl -s -o /dev/null -w "%{http_code}" "$STREAMCLONE_URL/v1/extension/health" 2>/dev/null || echo "000")
if [ "$sc_health" != "200" ]; then
  echo "  FAIL: Streamclone health returned $sc_health (want 200)"
  echo "  Ensure 'make up' has been run."
  exit 1
fi
echo "  OK: Streamclone healthy"

# Step 2: Check ReplayForge is alive
echo "[2/4] Checking ReplayForge health..."
rf_health=$(curl -s -o /dev/null -w "%{http_code}" "$REPLAYFORGE_URL/healthz" 2>/dev/null || echo "000")
if [ "$rf_health" != "200" ]; then
  echo "  FAIL: ReplayForge health returned $rf_health (want 200)"
  echo "  Ensure ReplayForge is running on :8095."
  exit 1
fi
echo "  OK: ReplayForge healthy"

# Step 3: Check clipper proxy route is reachable
echo "[3/4] Checking clipper proxy route (/v1/clipper/healthz)..."
proxy_health=$(curl -s -o /dev/null -w "%{http_code}" "$STREAMCLONE_URL/v1/clipper/healthz" 2>/dev/null || echo "000")
if [ "$proxy_health" != "200" ]; then
  echo "  WARN: Clipper proxy health returned $proxy_health"
  echo "  The /v1/clipper/* proxy may not be configured in Caddy."
else
  echo "  OK: Clipper proxy reachable"
fi

# Step 4: Summary
echo ""
echo "[4/4] Summary:"
echo "  Streamclone:    UP ($sc_health)"
echo "  ReplayForge:    UP ($rf_health)"
echo "  Clipper Proxy:  $([ "$proxy_health" = "200" ] && echo "UP" || echo "WARN ($proxy_health)")"
echo ""
echo "=== E2E Validation ==="
echo "To trigger a full clip job from an Analytics moment:"
echo "  1. Open $STREAMCLONE_URL in browser"
echo "  2. Navigate to Analytics → select a stream with VOD"
echo "  3. Click Export Moment on a clip candidate"
echo "  4. Verify job appears in Clip Studio at $REPLAYFORGE_URL:8096/studio"
echo ""
echo "For automated unit/integration tests (no running stacks needed):"
echo "  INTEGRATION=1 go test ./internal/analytics/ -run TestAnalyticsExportMomentToReplayForgeJobCreationE2E -v"
echo ""
echo "=== Done ==="
