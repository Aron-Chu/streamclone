#!/usr/bin/env bash
# Wait for streamclone-scraper, probe a TwitchTracker detail page, and auto-recover
# from common Camoufox failures (stale profile locks, dead browser pool).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CHECK_ONLY=false
MAX_FIX_ATTEMPTS=2
SCRAPE_URL="${SCRAPE_URL:-https://twitchtracker.com/jynxzi/streams/318832886110}"
SCRAPE_SECOND_URL="${SCRAPE_SECOND_URL:-https://twitchtracker.com/ishowspeed/streams/318098150359}"
SCRAPE_TIMEOUT_MS="${SCRAPE_TIMEOUT_MS:-120000}"

usage() {
  cat <<'EOF'
Usage: scripts/scraper-preflight.sh [options]

  --check-only     Probe scrape only; do not clear locks or recreate the container
  --url URL        TwitchTracker stream detail URL to probe
  --second-url URL Second detail URL; catches pooled-browser failures after the first scrape
  -h, --help       Show this help

Env:
  SCRAPER_SKIP_PREFLIGHT=1   make up-scraper / up-full skip this script
  SCRAPE_TIMEOUT_MS          Scrape timeout (default 120000)
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --check-only) CHECK_ONLY=true; shift ;;
    --url) SCRAPE_URL="$2"; shift 2 ;;
    --second-url) SCRAPE_SECOND_URL="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
done

fail() {
  echo "scraper-preflight: $1" >&2
  exit 1
}

# Docker Desktop on Windows: WSL bash may not see containers started from PowerShell.
docker_cmd() {
  if command -v docker.exe >/dev/null 2>&1; then
    docker.exe "$@"
  else
    docker "$@"
  fi
}

compose_scraper() {
  docker_cmd compose --env-file .env \
    -f deploy/docker-compose.yml \
    -f deploy/docker-compose.local-tunnel.yml \
    --profile scraper "$@"
}

wait_scraper_health() {
  echo "Waiting for scraper /health..."
  for i in $(seq 1 45); do
    if docker_cmd exec streamclone-scraper curl -sf http://127.0.0.1:8000/health >/dev/null 2>&1; then
      echo "  scraper healthy"
      return 0
    fi
    sleep 2
  done
  fail "scraper health check timed out — is streamclone-scraper running?"
}

run_scrape_probe() {
  local url="$1"
  local script="$ROOT/scripts/scrape-test-inline.py"
  docker_cmd cp "$script" streamclone-scraper:/tmp/scrape-probe.py >/dev/null
  docker_cmd exec \
    -e SCRAPE_URL="$url" \
    -e SCRAPE_TIMEOUT_MS="$SCRAPE_TIMEOUT_MS" \
    -e USE_PROXY=false \
    streamclone-scraper python /tmp/scrape-probe.py
}

run_sequential_scrape_probe() {
  local urls=("$SCRAPE_URL")
  if [ -n "$SCRAPE_SECOND_URL" ] && [ "$SCRAPE_SECOND_URL" != "$SCRAPE_URL" ]; then
    urls+=("$SCRAPE_SECOND_URL")
  fi

  local total="${#urls[@]}"
  local idx=1
  local url
  for url in "${urls[@]}"; do
    echo "Probing TwitchTracker scrape $idx/$total ($url)..."
    run_scrape_probe "$url"
    idx=$((idx + 1))
  done
}

clear_profile_locks() {
  echo "Clearing Camoufox profile locks in scraper volume..."
  docker_cmd exec streamclone-scraper python -c \
    'from profile_sync import clear_firefox_profile_locks; print("removed:", clear_firefox_profile_locks("/data/camoufox-profile"))'
}

recreate_scraper() {
  echo "Recreating streamclone-scraper..."
  compose_scraper up -d --no-deps --force-recreate scraper
  wait_scraper_health
}

print_recovery_hints() {
  cat <<'EOF'

Scraper preflight failed. Camoufox (headless Firefox) could not fetch TwitchTracker minute data.

Try:
  1. make scraper-reload          # force-recreate scraper container
  2. scripts/warm-camoufox-profile.ps1   # pass Cloudflare once; persists cookies
  3. make scraper-check           # re-run probe only

CDP fallback (Windows only, if Camoufox keeps dying in Docker):
  - Uncomment CDP lines in .env.local, run scripts/scraper-cdp.ps1, make scraper-reload

EOF
}

if ! docker_cmd ps --format '{{.Names}}' | grep -qx streamclone-scraper; then
  fail "streamclone-scraper is not running — start with: make up-scraper"
fi

wait_scraper_health

attempt=0
while true; do
  set +e
  probe_out="$(run_sequential_scrape_probe 2>&1)"
  probe_rc=$?
  set -e
  echo "$probe_out"

  if [ "$probe_rc" -eq 0 ]; then
    echo "scraper-preflight: Camoufox sequential scrape ok (meta#ecs or chart data present)"
    exit 0
  fi

  if [ "$CHECK_ONLY" = true ]; then
    print_recovery_hints
    fail "scrape probe failed (--check-only)"
  fi

  if [ "$attempt" -ge "$MAX_FIX_ATTEMPTS" ]; then
    print_recovery_hints
    fail "scrape probe failed after $MAX_FIX_ATTEMPTS recovery attempts"
  fi

  if echo "$probe_out" | grep -qiE 'browser has been closed|firefox is already running|parentlock|profile.*lock'; then
    echo "Detected Camoufox profile/browser issue — auto-recovering (attempt $((attempt + 1))/$MAX_FIX_ATTEMPTS)..."
    clear_profile_locks
    recreate_scraper
    attempt=$((attempt + 1))
    continue
  fi

  if echo "$probe_out" | grep -qiE 'cloudflare|just a moment|403'; then
    print_recovery_hints
    fail "Cloudflare blocked the scrape — warm the Camoufox profile (see above)"
  fi

  echo "Unhandled scrape failure — recreating scraper once..."
  recreate_scraper
  attempt=$((attempt + 1))
done
